import { describe, expect, test } from 'bun:test';
import { getNavigationEntry, getIndexedNavigationEntries } from './navigation';

const root = new URL('../content/docs/sdk/easy/', import.meta.url);
const revision = '8a3218cd5c9465f06df371f040a3641188bd4900';
const repository = 'https://github.com/WuKongIM/WuKongEasySDK-CPP';

describe('C++ EasySDK source tutorial', () => {
  test('publishes both locales and pins source independently of registry packages', async () => {
    for (const [locale, suffix] of [['zh', ''], ['en', '.en']] as const) {
      const page = await Bun.file(new URL(`cpp/getting-started${suffix}.mdx`, root)).text();
      const overview = await Bun.file(new URL(`index${suffix}.mdx`, root)).text();
      const examples = await Bun.file(new URL(`examples${suffix}.mdx`, root)).text();
      expect(getNavigationEntry(locale, 'sdk', ['easy', 'cpp', 'getting-started'])?.status).toBe('published');
      expect(getIndexedNavigationEntries(locale).map((entry) => entry.url)).toContain(`/${locale}/sdk/easy/cpp/getting-started`);
      for (const content of [page, overview, examples]) {
        expect(content).toContain(repository);
        expect(content).toContain(revision);
        expect(content).not.toContain('CPP_SOURCE_REV');
        expect(content).toContain('0.1.0');
      }
      for (const content of [overview, examples]) {
        expect(content).toContain(`/${locale}/sdk/easy/cpp/getting-started`);
      }
      expect(page).toMatch(/没有预编译包或 Registry 发布|no prebuilt package or registry release/);
      expect(page).toContain(`git checkout ${revision}`);
      expect(page).toContain('WuKongEasySDK::WuKongEasySDK');
      expect(page).toContain('CMAKE_TOOLCHAIN_FILE');
      expect(page).toContain('ctest --test-dir build');
      expect(page).toContain('im.connect().get()');
      expect(page).toContain('im.send(');
      expect(page).toContain('im.off(messageListener)');
      expect(page).toContain('im.disconnect().get()');
      expect(page).toContain('im.destroy().get()');
      expect(page).toContain('ErrorCode::QueueFull');
      expect(page).toContain('result: null');
      expect(page).toContain('Options::caFile');
      expect(page).toContain('clientMsgNo');
      expect(page).toMatch(/不能在回调中等待 SDK future|must never wait on an SDK future/);
      expect(page).toMatch(/APP `0`.*WEB `1`.*PC\/Desktop `2`/);
      expect(page).toContain(`/${locale}/guide/integration/authentication`);
      expect(page).toContain(`/${locale}/guide/integration/messaging`);
    }
  });
});
