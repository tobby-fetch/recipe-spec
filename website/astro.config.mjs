import { defineConfig } from "astro/config";

// GitHub Pages project site for the tobby-fetch/recipe-spec repository.
// Served at https://tobby-fetch.github.io/recipe-spec/
// If a custom domain is assigned later, set `site` to it and remove `base`.
export default defineConfig({
  site: "https://tobby-fetch.github.io",
  base: "/recipe-spec",
  markdown: {
    // Code blocks follow the visitor's color scheme; the switching CSS
    // lives in src/layouts/Guide.astro.
    shikiConfig: {
      themes: { light: "github-light", dark: "github-dark" },
    },
  },
});
