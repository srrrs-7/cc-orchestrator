import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Vite requires a default export from its config file; this is the
// documented exception to the "named export only" rule.
export default defineConfig({
  plugins: [react(), tailwindcss()],
});
