import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

// yarn dev serves the editor with hot reload; yarn build writes the production
// assets to dist, where Go embeds them in the Sciurus binary.
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  server: {
    proxy: {
      "/api": {
        target: "http://127.0.0.1:7480",
        changeOrigin: true,
        configure(proxy) {
          // The development server terminates the browser's origin. Forward its
          // requests as local CLI requests, preserving production origin checks.
          proxy.on("proxyReq", (request) => request.removeHeader("origin"));
        },
      },
    },
  },
  // Fonts must remain files: the Go server's content policy permits same-origin
  // font URLs, without allowing inline data URLs.
  build: { target: "es2022", assetsInlineLimit: 0 },
});
