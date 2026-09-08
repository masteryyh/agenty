import { defineConfig } from "vite";

export default defineConfig({
    root: "web",
    server: {
        host: "127.0.0.1",
        port: 5173,
        strictPort: true,
        proxy: {
            "/api": {
                target: "http://127.0.0.1:" + (process.env.INSPECTOR_API_PORT ?? "4318"),
                changeOrigin: true,
            },
        },
    },
    build: { outDir: "dist", emptyOutDir: true },
});
