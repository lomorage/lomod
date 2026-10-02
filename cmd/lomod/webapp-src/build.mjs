import * as esbuild from 'esbuild';

const watch = process.argv.includes('--watch');

// ESM + code-splitting so gallery/livephoto.js (dynamic import()) becomes its
// own chunk, only fetched when a Live Photo is actually opened.
const galleryOpts = {
  entryPoints: { gallery: 'gallery/main.js' },
  bundle: true,
  minify: true,
  sourcemap: true,
  splitting: true,
  format: 'esm',
  target: ['es2020'],
  outdir: '../static/lomo/dist',
  logLevel: 'info',
};

// Classic (non-module) worker: `new Worker(...)` is called without
// `{ type: 'module' }`, and the script uses importScripts().
const workerOpts = {
  entryPoints: { 'argon2-worker': 'shared/argon2-worker.js' },
  bundle: true,
  minify: true,
  sourcemap: true,
  format: 'iife',
  target: ['es2020'],
  outdir: '../static/lomo/dist',
  logLevel: 'info',
};

if (watch) {
  const [galleryCtx, workerCtx] = await Promise.all([
    esbuild.context(galleryOpts),
    esbuild.context(workerOpts),
  ]);
  await Promise.all([galleryCtx.watch(), workerCtx.watch()]);
  console.log('watching for changes...');
} else {
  await Promise.all([esbuild.build(galleryOpts), esbuild.build(workerOpts)]);
}
