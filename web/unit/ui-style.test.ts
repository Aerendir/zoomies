import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { runInNewContext } from 'node:vm';
import { resolveUIStyle, UI_STYLE_KEY } from '../src/lib/state/ui-style';
import { assertLook, expectedLook, storedLook } from '../tests/support/look.mjs';

const html = readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const bootstrap = /<script>([\s\S]*?)<\/script>/.exec(html)?.[1];
assert.ok(bootstrap);

function boot(style: unknown, mode: string | null = null, blocked = false) {
  const attrs = new Map<string, string>();
  const tags = ['light', 'dark'].map((mode) => {
    const markup = [...html.matchAll(/<meta\b[^>]*>/g)]
      .map((match) => match[0])
      .find((markup) => markup.includes(`data-theme-colour="${mode}"`));
    assert.ok(markup);
    const data = new Map([...markup.matchAll(/([\w-]+)="([^"]*)"/g)].map((m) => [m[1], m[2]]));
    return {
      getAttribute: (key: string) => data.get(key),
      setAttribute: (key: string, value: string) => data.set(key, value),
      data,
    };
  });
  runInNewContext(bootstrap!, {
    localStorage: {
      getItem(key: string) {
        if (blocked) throw new Error('Blocked');
        return key === UI_STYLE_KEY ? style : key === 'zoomies.theme' ? mode : null;
      },
    },
    document: {
      documentElement: { setAttribute: (key: string, value: string) => attrs.set(key, value) },
      querySelectorAll: () => tags,
    },
    window: { matchMedia: () => ({ matches: false }) },
    matchMedia: () => ({ matches: false }),
  });
  return { attrs, tags };
}

test('unset and unknown visual styles get Mission Control; only a stored classic opts out', () => {
  for (const value of [undefined, null, '', 'dark', 'MISSION-CONTROL', {}, '<style>']) {
    assert.equal(resolveUIStyle(value), 'mission-control');
    assert.equal(boot(value).attrs.get('data-ui-style'), 'mission-control');
  }
  assert.equal(resolveUIStyle('classic'), 'classic');
  assert.equal(boot('classic').attrs.has('data-ui-style'), false);
});

test('an unset colour mode boots dark', () => {
  const { attrs, tags } = boot(null, null);
  assert.equal(attrs.get('data-theme'), 'dark');
  for (const tag of tags) {
    assert.equal(
      tag.data.get('media'),
      tag.data.get('data-theme-colour') === 'dark' ? 'all' : 'not all',
    );
  }
});

test('the stored style and explicit colour mode apply together before first paint', () => {
  for (const mode of ['light', 'dark']) {
    const { attrs, tags } = boot('mission-control', mode);
    assert.equal(attrs.get('data-ui-style'), 'mission-control');
    assert.equal(attrs.get('data-theme'), mode);
    for (const tag of tags) {
      assert.equal(tag.data.get('content'), tag.data.get('data-mission-control-colour'));
      assert.equal(
        tag.data.get('media'),
        tag.data.get('data-theme-colour') === mode ? 'all' : 'not all',
      );
    }
  }
});

test('system mode keeps its native media queries independently of visual style', () => {
  for (const style of ['classic', 'mission-control']) {
    const { attrs, tags } = boot(style, 'system');
    assert.equal(attrs.has('data-theme'), false);
    for (const tag of tags) {
      assert.equal(
        tag.data.get('media'),
        `(prefers-color-scheme: ${tag.data.get('data-theme-colour')})`,
      );
    }
  }
});

test('blocked storage boots the defaults, the same look the app state resolves to', () => {
  const { attrs } = boot('classic', 'light', true);
  assert.equal(attrs.get('data-ui-style'), 'mission-control');
  assert.equal(attrs.get('data-theme'), 'dark');
});

test('browser-chrome colours match the actual palette tokens in both styles', () => {
  const css = readFileSync(new URL('../src/lib/styles/tokens.css', import.meta.url), 'utf8');
  const mission = css.slice(css.indexOf('/* Style previews and Mission Control'));
  for (const mode of ['light', 'dark']) {
    const { tags } = boot('mission-control', mode);
    const tag = tags.find((tag) => tag.data.get('data-theme-colour') === mode)!;
    const classicScope =
      mode === 'light' ? css : css.slice(css.indexOf(":root[data-theme='dark']"));
    const missionScope =
      mode === 'light' ? mission : mission.slice(mission.indexOf(":root[data-theme='dark']"));
    assert.equal(
      tag.data.get('data-classic-colour'),
      /--z-bg:\s*(#[\da-f]{6});/.exec(classicScope)?.[1],
    );
    assert.equal(
      tag.data.get('data-mission-control-colour'),
      /--z-mission-bg:\s*(#[\da-f]{6});/.exec(missionScope)?.[1],
    );
  }
});

// The picture runner (tests/support/screenshots.mjs) photographs one look per
// pass. An empty browser is dark Mission Control whatever the system says, so a
// pass has to store the look it means, and this holds it to the bootstrap it relies
// on: the pass that said light, or Classic, once photographed neither.
test('a capture pass opens the page on the look it is photographing', () => {
  for (const scheme of ['light', 'dark'] as const) {
    for (const style of ['classic', 'mission-control'] as const) {
      const stored = storedLook(scheme, style);
      const { attrs } = boot(stored['zoomies.ui-style'], stored['zoomies.theme']);
      const want = expectedLook(scheme, style);
      assert.equal(attrs.get('data-theme') ?? null, want.theme, `${scheme} ${style}: theme`);
      assert.equal(attrs.get('data-ui-style') ?? null, want.style, `${scheme} ${style}: style`);
    }
  }
});

test('a pass whose page opened on another look stops instead of filing the picture', async () => {
  const opened = (theme: string | null, style: string | null) => ({
    evaluate: async () => ({ theme, style }),
  });
  await assertLook(opened('light', null), 'overview', 'light', 'classic');
  await assertLook(opened('dark', 'mission-control'), 'overview', 'dark', 'mission-control');
  await assert.rejects(
    assertLook(opened('dark', 'mission-control'), 'overview', 'light', 'classic'),
    /overview opened on dark mission-control but this pass is light classic/,
  );
});
