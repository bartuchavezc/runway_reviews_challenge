import { defineConfig } from 'vitest/config'

// Kept separate from vite.config.ts because Vite 8 (rolldown) and Vitest 3 ship
// incompatible Plugin types — putting the React plugin and the test config in
// the same file triggers a type mismatch. The current tests are pure functions
// and don't need the React plugin to run.
export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: false,
  },
})
