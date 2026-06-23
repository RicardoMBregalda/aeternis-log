// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// Project site on GitHub Pages → served under /aeternis-log/.
// Change `site` + add public/CNAME to move to a custom domain later.
export default defineConfig({
  site: 'https://RicardoMBregalda.github.io',
  base: '/aeternis-log/',
  trailingSlash: 'always',
  integrations: [
    starlight({
      title: 'AeternisLog',
      description:
        'AeternisLog — tamper-evident data anchoring. Give any data cryptographic proof of integrity with Merkle trees anchored on Hyperledger Fabric.',
      logo: { src: './src/assets/logo.svg', alt: 'AeternisLog' },
      favicon: '/favicon.svg',
      customCss: ['./src/styles/custom.css'],
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/RicardoMBregalda/aeternis-log',
        },
      ],
      editLink: {
        baseUrl:
          'https://github.com/RicardoMBregalda/aeternis-log/edit/main/website/',
      },
      lastUpdated: true,
      // Code blocks adapt to the site theme (dark on dark, light on light).
      expressiveCode: {
        themes: ['github-dark', 'github-light'],
        styleOverrides: { borderRadius: '12px', codeFontFamily: 'var(--sl-font-mono)' },
      },
      components: {
        // Custom segmented light/dark/system toggle (replaces the default select).
        ThemeSelect: './src/components/ThemeSelect.astro',
      },
      head: [
        {
          tag: 'meta',
          attrs: { property: 'og:image', content: 'https://RicardoMBregalda.github.io/aeternis-log/og.png' },
        },
        {
          tag: 'meta',
          attrs: { name: 'twitter:card', content: 'summary_large_image' },
        },
      ],
      sidebar: [
        { label: 'Introduction', autogenerate: { directory: 'introduction' } },
        { label: 'Getting started', autogenerate: { directory: 'getting-started' } },
        { label: 'Guides', autogenerate: { directory: 'guides' } },
        { label: 'API reference', autogenerate: { directory: 'api' } },
        { label: 'SDKs', autogenerate: { directory: 'sdks' } },
        { label: 'Self-hosting & operations', autogenerate: { directory: 'operations' } },
        { label: 'Security & compliance', autogenerate: { directory: 'security' } },
        { label: 'Commercial', autogenerate: { directory: 'commercial' } },
        { label: 'Reference', autogenerate: { directory: 'reference' } },
      ],
    }),
  ],
});
