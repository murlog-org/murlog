import { defineConfig } from 'vitepress'
import { withMermaid } from 'vitepress-plugin-mermaid'

export default withMermaid(
  defineConfig({
    title: 'murlog docs',
    description: '1人1インスタンスの ActivityPub マイクロブログ',
    lang: 'ja',

    themeConfig: {
      search: {
        provider: 'local',
      },

      nav: [
        { text: 'プロダクト概要', link: '/overview' },
        { text: 'murlog API', link: '/murlog-api' },
      ],

      sidebar: [
        { text: 'プロダクト概要', link: '/overview' },
        { text: 'ドメインモデル', link: '/domain' },
        { text: 'サーバーアーキテクチャ', link: '/architecture' },
        { text: 'Web フロントエンド', link: '/frontend' },
        { text: 'CGI 動作アーキテクチャ', link: '/cgi' },
        { text: '多言語対応', link: '/i18n' },
        { text: '認証', link: '/auth' },
        { text: 'murlog API', link: '/murlog-api' },
        {
          text: 'ActivityPub',
          link: '/activitypub/',
          collapsed: false,
          items: [
            { text: 'Actor', link: '/activitypub/actor' },
            {
              text: 'Activity',
              link: '/activitypub/activity/',
              collapsed: false,
              items: [
                { text: 'Follow', link: '/activitypub/activity/follow' },
                { text: 'Create', link: '/activitypub/activity/create' },
                { text: 'Delete', link: '/activitypub/activity/delete' },
                { text: 'Block', link: '/activitypub/activity/block' },
              ],
            },
            { text: 'HTTP Signature', link: '/activitypub/http-signature' },
            { text: 'WebFinger・NodeInfo', link: '/activitypub/webfinger' },
            { text: 'fediverse 互換性', link: '/activitypub/compatibility' },
          ],
        },
        { text: 'セキュリティ', link: '/security' },
        {
          text: '開発',
          collapsed: false,
          items: [
            { text: 'CSS 設計', link: '/css-design' },
            { text: 'テスト', link: '/testing' },
            { text: 'ベンチマーク', link: '/benchmark' },
          ],
        },
      ],

      socialLinks: [
        { icon: 'github', link: 'https://github.com/murlog-org/murlog' },
      ],
    },
  })
)
