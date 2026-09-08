# Merge the separately cataloged embedded executable into the image SBOM.
# Its directory-source root is not an image component; omit that synthetic edge.
if ($child | length) != 1 or $child[0].bomFormat != "CycloneDX" or
   ($child[0].components | type) != "array" or
   ($child[0].components | length) == 0 then
  error("missing embedded Prometheus SBOM components")
else
  .components = ((.components + $child[0].components) | unique_by(."bom-ref")) |
  .dependencies = (
    ((.dependencies // []) + (($child[0].dependencies // []) |
      map(select(.ref != $child[0].metadata.component."bom-ref")))) |
    group_by(.ref) |
    map(.[0] + {dependsOn: ([.[].dependsOn[]?] | unique)})
  )
end
