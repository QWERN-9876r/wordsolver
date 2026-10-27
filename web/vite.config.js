import { defineConfig } from 'vite';
import { rook } from 'rooklang/vite';

export default defineConfig({
  // Relative asset paths, so the build works from any folder or static host.
  base: './',
  plugins: [rook()],
  // The dictionary, words.txt, lives in the Go project one level up.
  server: { fs: { allow: ['..'] } },
});
