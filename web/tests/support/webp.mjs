// Lossless WebP encoding for the committed captures, through Pillow.
//
// The browser can write WebP itself, but its encoder compresses these pages
// several times worse than Pillow's, and a documentation gallery that is
// hundreds of kilobytes per page stops being worth committing. Pillow's
// lossless mode keeps every pixel while making a full refresh practical, so
// the browser writes PNGs to a temporary directory and this turns them into
// the files that are committed.
//
// `npm` does not install this: the site build already needs Python, and
// `pip install pillow` is the one prerequisite, named in docs/ui-guidelines.md.

import { spawnSync } from 'node:child_process';

/** Encode every PNG in `pngDir` as lossless WebP in `outDir`. */
export function encodeWebp(pngDir, outDir) {
  const script = [
    'import pathlib, sys',
    'from PIL import Image',
    'src, dst = map(pathlib.Path, sys.argv[1:3])',
    'for png in sorted(src.glob("*.png")):',
    '    out = dst / (png.stem + ".webp")',
    // Lossless WebP's quality controls compression effort, not image quality.
    // This keeps every pixel while making a full-gallery refresh practical.
    '    Image.open(png).convert("RGB").save(out, lossless=True, quality=80, method=4)',
    '    print(f"  {out.name}  {out.stat().st_size // 1024} KB")',
  ].join('\n');
  const run = spawnSync('python3', ['-u', '-c', script, pngDir, outDir], { stdio: 'inherit' });
  if (run.error || run.status !== 0) {
    throw new Error(
      'encoding the screenshots needs Python 3 with Pillow: pip install pillow\n' +
        `(python3 ${run.error ? `could not start: ${run.error.message}` : `exited ${run.status}`})`,
    );
  }
}
