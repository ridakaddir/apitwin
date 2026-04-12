import { defineConfig } from 'vitepress'

// https://vitepress.dev/reference/site-config
export default defineConfig({
  // Read markdown content from the top-level `docs/` tree (one source of
  // truth for both GitHub's native renderer and the VitePress site). All
  // Node tooling stays under `website/`; `docs/` contains only markdown.
  srcDir: '../docs',

  // Dead-link validation is disabled: some docs link out to the `examples/`
  // code directory (e.g. `../examples/grpc-wrap`) which VitePress doesn't
  // serve. Those links are intentional and render correctly on GitHub.
  ignoreDeadLinks: true,

  // `docs/README.md` is the GitHub-facing TOC; the VitePress home page lives
  // in `docs/index.md`. Exclude README.md so VitePress doesn't try to build
  // both as conflicting routes.
  srcExclude: ['README.md'],

  // Every directory uses `README.md` as its index (GitHub convention). Rewrite
  // any nested `README.md` to `index.md` at build time so VitePress treats
  // `docs/features/README.md` as the `/features/` landing page. The top-level
  // `docs/README.md` is excluded above and doesn't hit this rewrite.
  rewrites(id) {
    return id.replace(/(^|\/)README\.md$/, '$1index.md')
  },

  markdown: {
    theme: 'github-dark-default',
    // Auto-apply `v-pre` to every inline `code` token so Vue doesn't try to
    // interpret mustache syntax like `{{uuid}}` or `{{ref:...}}` that appears
    // throughout the docs. Fenced code blocks are already `v-pre` by default.
    // This removes the need for any per-file `<code v-pre>…</code>` wrapping.
    config: (md) => {
      const defaultRender = md.renderer.rules.code_inline!
      md.renderer.rules.code_inline = (tokens, idx, options, env, self) => {
        tokens[idx].attrSet('v-pre', '')
        return defaultRender(tokens, idx, options, env, self)
      }
    }
  },
  title: 'apitwin',
  description: 'A fast, zero-dependency CLI tool for mocking, stubbing, and proxying HTTP and gRPC APIs',
  base: '/apitwin/',
  head: [
    ['link', { rel: 'icon', href: '/apitwin/favicon.ico' }]
  ],

  themeConfig: {
    // https://vitepress.dev/reference/default-theme-config
    logo: {
      light: '/logo.svg',
      dark: '/logo-dark.svg'
    },

    nav: [
      { text: 'Home', link: '/' },
      { text: 'Guide', link: '/installation' },
      { text: 'Configuration', link: '/configuration/' },
      { text: 'Features', link: '/features/' },
      { text: 'gRPC', link: '/grpc/' },
      { text: 'OpenAPI', link: '/openapi/' },
      { text: 'Examples', link: '/examples' }
    ],

    sidebar: [
      {
        text: 'Getting Started',
        collapsed: false,
        items: [
          { text: 'Installation', link: '/installation' },
          { text: 'Quick Start', link: '/quick-start' },
          { text: 'CLI Reference', link: '/cli-reference' },
          { text: 'Examples', link: '/examples' }
        ]
      },
      {
        text: 'Configuration',
        collapsed: false,
        items: [
          { text: 'Overview', link: '/configuration/' },
          { text: 'Routes', link: '/configuration/routes' },
          { text: 'Cases', link: '/configuration/cases' },
          { text: 'Config Formats', link: '/configuration/formats' }
        ]
      },
      {
        text: 'Features',
        collapsed: true,
        items: [
          { text: 'Overview', link: '/features/' },
          { text: 'Conditions', link: '/features/conditions' },
          { text: 'Named Parameters', link: '/features/named-parameters' },
          { text: 'Directory-Based Stubs', link: '/features/directory-stubs' },
          { text: 'Cascade Mutations', link: '/features/cascade-mutations' },
          { text: 'Dynamic File Resolution', link: '/features/dynamic-files' },
          { text: 'Template Tokens', link: '/features/template-tokens' },
          { text: 'Cross-Endpoint References', link: '/features/cross-endpoint-references' },
          { text: 'Array Processing', link: '/features/array-processing' },
          { text: 'Type Conversion', link: '/features/type-conversion' },
          { text: 'Response Transitions', link: '/features/response-transitions' },
          { text: 'Runtime State Isolation', link: '/features/runtime-state' },
          { text: 'Record Mode', link: '/features/record-mode' },
          { text: 'Hot Reload', link: '/features/hot-reload' },
          { text: 'API Prefix', link: '/features/api-prefix' },
          { text: 'Devtool UI', link: '/features/devtool-ui' }
        ]
      },
      {
        text: 'gRPC',
        collapsed: true,
        items: [
          { text: 'Overview', link: '/grpc/' },
          { text: 'Quick Start', link: '/grpc/quick-start' },
          { text: 'Configuration', link: '/grpc/config' },
          { text: 'Stubs & Conditions', link: '/grpc/stubs' },
          { text: 'Persistence', link: '/grpc/persistence' },
          { text: 'Generation', link: '/grpc/generation' }
        ]
      },
      {
        text: 'OpenAPI',
        collapsed: true,
        items: [
          { text: 'Overview', link: '/openapi/' },
          { text: 'Generate Command', link: '/openapi/generate' },
          { text: 'Stub Quality', link: '/openapi/stub-quality' }
        ]
      }
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/ridakaddir/apitwin' },
      { icon: 'npm', link: 'https://www.npmjs.com/package/@ridakaddir/apitwin' }
    ],

    search: {
      provider: 'local'
    },

    editLink: {
      pattern: 'https://github.com/ridakaddir/apitwin/edit/main/docs/:path'
    },

    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright © 2026 Rida Kaddir'
    }
  }
})