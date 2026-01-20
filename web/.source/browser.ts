// @ts-nocheck
import { browser } from 'fumadocs-mdx/runtime/browser';
import type * as Config from '../source.config';

const create = browser<typeof Config, import("fumadocs-mdx/runtime/types").InternalTypeConfig & {
  DocData: {
  }
}>();
const browserCollections = {
  docs: create.doc("docs", {"api-introduction.mdx": () => import("../content/docs/api-introduction.mdx?collection=docs"), "authentication.mdx": () => import("../content/docs/authentication.mdx?collection=docs"), "index.mdx": () => import("../content/docs/index.mdx?collection=docs"), "plans-limits.mdx": () => import("../content/docs/plans-limits.mdx?collection=docs"), "quickstart.mdx": () => import("../content/docs/quickstart.mdx?collection=docs"), "sdks/curl.mdx": () => import("../content/docs/sdks/curl.mdx?collection=docs"), "sdks/go.mdx": () => import("../content/docs/sdks/go.mdx?collection=docs"), "sdks/index.mdx": () => import("../content/docs/sdks/index.mdx?collection=docs"), "sdks/python.mdx": () => import("../content/docs/sdks/python.mdx?collection=docs"), "sdks/rust.mdx": () => import("../content/docs/sdks/rust.mdx?collection=docs"), "sdks/typescript.mdx": () => import("../content/docs/sdks/typescript.mdx?collection=docs"), "guides/crawling.mdx": () => import("../content/docs/guides/crawling.mdx?collection=docs"), "guides/extraction.mdx": () => import("../content/docs/guides/extraction.mdx?collection=docs"), "guides/index.mdx": () => import("../content/docs/guides/index.mdx?collection=docs"), "guides/schemas.mdx": () => import("../content/docs/guides/schemas.mdx?collection=docs"), "guides/webhooks.mdx": () => import("../content/docs/guides/webhooks.mdx?collection=docs"), }),
};
export default browserCollections;