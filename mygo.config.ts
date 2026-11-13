import { defineConfig } from "mygo-cli";

export default defineConfig({
  name: "SnapBridge",
  identifier: "com.fanweiya.snapbridge",
  version: "0.1.0",
  icon: "resources/icon.png",
  // `mygo dev` runs devCommand and loads devUrl; `mygo build` runs
  // buildCommand and embeds frontendDist into the app.
  devUrl: "http://localhost:5173",
  devCommand: "bun run dev:web",
  buildCommand: "bun run build:web",
  frontendDist: "dist",
  bindings: "src/mygo.ts",
  out: "build",
});
