package scripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDockerPrometheusSBOMMergesSharedDependencies(t *testing.T) {
	child := filepath.Join(t.TempDir(), "child.json")
	require.NoError(t, os.WriteFile(child, []byte(`{
	  "bomFormat":"CycloneDX","metadata":{"component":{"bom-ref":"directory"}},
	  "components":[{"bom-ref":"prometheus"},{"bom-ref":"shared"},{"bom-ref":"child-lib"}],
	  "dependencies":[
	    {"ref":"directory","dependsOn":["prometheus"]},
	    {"ref":"prometheus","dependsOn":["shared"]},
	    {"ref":"shared","dependsOn":["child-lib"]}
	  ]
	}`), 0o600))
	cmd := exec.Command("jq", "--slurpfile", "child", child, "-f",
		filepath.Join(repoRoot(t), "scripts", "merge-docker-prometheus-sbom.jq"))
	cmd.Stdin = strings.NewReader(`{
	  "bomFormat":"CycloneDX","metadata":{"component":{"bom-ref":"image"}},
	  "components":[{"bom-ref":"wukongim"},{"bom-ref":"shared"},{"bom-ref":"image-lib"}],
	  "dependencies":[{"ref":"shared","dependsOn":["image-lib"]}]
	}`)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	var result struct {
		Components []struct {
			Ref string `json:"bom-ref"`
		} `json:"components"`
		Dependencies []struct {
			Ref       string   `json:"ref"`
			DependsOn []string `json:"dependsOn"`
		} `json:"dependencies"`
	}
	require.NoError(t, json.Unmarshal(output, &result))
	require.Len(t, result.Components, 5)
	require.Len(t, result.Dependencies, 2)
	for _, dependency := range result.Dependencies {
		require.NotEqual(t, "directory", dependency.Ref)
		if dependency.Ref == "shared" {
			require.ElementsMatch(t, []string{"image-lib", "child-lib"}, dependency.DependsOn)
		}
	}
}

func TestDockerPrometheusSBOMRejectsMissingChildInventory(t *testing.T) {
	child := filepath.Join(t.TempDir(), "child.json")
	require.NoError(t, os.WriteFile(child, []byte(`{"bomFormat":"CycloneDX","components":[]}`), 0o600))
	cmd := exec.Command("jq", "--slurpfile", "child", child, "-f",
		filepath.Join(repoRoot(t), "scripts", "merge-docker-prometheus-sbom.jq"))
	cmd.Stdin = strings.NewReader(`{"components":[]}`)
	output, err := cmd.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "missing embedded Prometheus SBOM components")
}
