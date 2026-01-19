import type { ComponentType } from 'react';
import { source } from '@/lib/source';
import {
  DocsPage,
  DocsBody,
  DocsDescription,
  DocsTitle,
} from 'fumadocs-ui/page';
import { notFound } from 'next/navigation';
import defaultMdxComponents from 'fumadocs-ui/mdx';
import { APIPage } from '@/components/api-page';
import { RefyneText } from '@/components/refyne-logo';
import { TierLimitsTable } from '@/components/tier-limits-table';

export default async function Page(props: {
  params: Promise<{ slug?: string[] }>;
}) {
  const params = await props.params;
  const page = source.getPage(params.slug ?? []);
  if (!page) notFound();

  const { data } = page;

  // Handle dynamic OpenAPI pages (from openapiSource - have type: 'openapi')
  if (data.type === 'openapi') {
    const apiPageProps = (data as { getAPIPageProps: () => Parameters<typeof APIPage>[0] }).getAPIPageProps();
    return (
      <DocsPage toc={data.toc} full>
        <DocsTitle>{data.title}</DocsTitle>
        <DocsDescription>{data.description}</DocsDescription>
        <DocsBody>
          <APIPage {...apiPageProps} />
        </DocsBody>
      </DocsPage>
    );
  }

  // Handle regular MDX pages (including generated API reference)
  const MDX = (data as { body: ComponentType<{ components: Record<string, unknown> }> }).body;

  return (
    <DocsPage toc={data.toc}>
      <DocsTitle>{data.title}</DocsTitle>
      <DocsDescription>{data.description}</DocsDescription>
      <DocsBody>
        <MDX components={{ ...defaultMdxComponents, APIPage, Refyne: RefyneText, TierLimitsTable }} />
      </DocsBody>
    </DocsPage>
  );
}

export async function generateStaticParams() {
  return source.generateParams();
}

export async function generateMetadata(props: {
  params: Promise<{ slug?: string[] }>;
}) {
  const params = await props.params;
  const page = source.getPage(params.slug ?? []);
  if (!page) notFound();

  return {
    title: page.data.title,
    description: page.data.description,
  };
}
