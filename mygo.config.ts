import { defineConfig } from "mygo-cli";

export default defineConfig({
  name: "SnapBridge",
  identifier: "com.fanweiya.snapbridge",
  version: "0.1.2",
  icon: "resources/icon.png",
  // `mygo dev` runs devCommand and loads devUrl; `mygo build` runs
  // buildCommand and embeds frontendDist into the app.
  devUrl: "http://localhost:5173",
  devCommand: "bun run dev:web",
  buildCommand: "bun run build:web",
  frontendDist: "dist",
  bindings: "src/mygo.ts",
  // 更新包由 mygo build 用 Ed25519 私钥签名，装好的应用只接受这把公钥签出的版本。
  // 私钥不进仓库，留在 %APPDATA%\mygo\update-keys\mygo-update.key。
  updates: {
    publicKey: "7VSdnRY8AraVMUPabVPTe2fIY1bjjRKhiQ62ifPNOQw=",
    github: "fanweiya/snapbridge",
  },
  out: "build",
});
