import { defineConfig } from 'vitepress'

// https://vitepress.dev/reference/site-config
export default defineConfig({
  ignoreDeadLinks: true,
  markdown: {
    theme: 'github-dark-default'
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