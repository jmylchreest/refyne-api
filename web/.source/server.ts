// @ts-nocheck
import * as __fd_glob_18 from "../content/docs/guides/webhooks.mdx?collection=docs"
import * as __fd_glob_17 from "../content/docs/guides/schemas.mdx?collection=docs"
import * as __fd_glob_16 from "../content/docs/guides/index.mdx?collection=docs"
import * as __fd_glob_15 from "../content/docs/guides/extraction.mdx?collection=docs"
import * as __fd_glob_14 from "../content/docs/guides/crawling.mdx?collection=docs"
import * as __fd_glob_13 from "../content/docs/sdks/typescript.mdx?collection=docs"
import * as __fd_glob_12 from "../content/docs/sdks/rust.mdx?collection=docs"
import * as __fd_glob_11 from "../content/docs/sdks/python.mdx?collection=docs"
import * as __fd_glob_10 from "../content/docs/sdks/index.mdx?collection=docs"
import * as __fd_glob_9 from "../content/docs/sdks/go.mdx?collection=docs"
import * as __fd_glob_8 from "../content/docs/sdks/curl.mdx?collection=docs"
import * as __fd_glob_7 from "../content/docs/quickstart.mdx?collection=docs"
import * as __fd_glob_6 from "../content/docs/plans-limits.mdx?collection=docs"
import * as __fd_glob_5 from "../content/docs/index.mdx?collection=docs"
import * as __fd_glob_4 from "../content/docs/authentication.mdx?collection=docs"
import * as __fd_glob_3 from "../content/docs/api-introduction.mdx?collection=docs"
import { default as __fd_glob_2 } from "../content/docs/sdks/meta.json?collection=docs"
import { default as __fd_glob_1 } from "../content/docs/guides/meta.json?collection=docs"
import { default as __fd_glob_0 } from "../content/docs/meta.json?collection=docs"
import { server } from 'fumadocs-mdx/runtime/server';
import type * as Config from '../source.config';

const create = server<typeof Config, import("fumadocs-mdx/runtime/types").InternalTypeConfig & {
  DocData: {
  }
}>({"doc":{"passthroughs":["extractedReferences"]}});

export const docs = await create.docs("docs", "content/docs", {"meta.json": __fd_glob_0, "guides/meta.json": __fd_glob_1, "sdks/meta.json": __fd_glob_2, }, {"api-introduction.mdx": __fd_glob_3, "authentication.mdx": __fd_glob_4, "index.mdx": __fd_glob_5, "plans-limits.mdx": __fd_glob_6, "quickstart.mdx": __fd_glob_7, "sdks/curl.mdx": __fd_glob_8, "sdks/go.mdx": __fd_glob_9, "sdks/index.mdx": __fd_glob_10, "sdks/python.mdx": __fd_glob_11, "sdks/rust.mdx": __fd_glob_12, "sdks/typescript.mdx": __fd_glob_13, "guides/crawling.mdx": __fd_glob_14, "guides/extraction.mdx": __fd_glob_15, "guides/index.mdx": __fd_glob_16, "guides/schemas.mdx": __fd_glob_17, "guides/webhooks.mdx": __fd_glob_18, });