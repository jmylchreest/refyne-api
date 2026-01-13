'use client';

import { usePlans } from '@clerk/nextjs/experimental';
import Link from 'next/link';
import { useEffect, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';

export function PricingSection() {
  const { data: plans, isLoading } = usePlans();
  const [descriptionHeight, setDescriptionHeight] = useState<number>(0);
  const [featuresHeight, setFeaturesHeight] = useState<number>(0);
  const descriptionRefs = useRef<(HTMLDivElement | null)[]>([]);
  const featuresRefs = useRef<(HTMLUListElement | null)[]>([]);

  // Calculate max heights after render
  useEffect(() => {
    if (!plans || plans.length === 0) return;

    // Measure description heights
    const descHeights = descriptionRefs.current
      .filter(Boolean)
      .map(el => el!.scrollHeight);
    const maxDescHeight = Math.max(...descHeights, 0);
    setDescriptionHeight(maxDescHeight);

    // Measure features heights
    const featHeights = featuresRefs.current
      .filter(Boolean)
      .map(el => el!.scrollHeight);
    const maxFeatHeight = Math.max(...featHeights, 0);
    setFeaturesHeight(maxFeatHeight);
  }, [plans]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-zinc-900 dark:border-white" />
      </div>
    );
  }

  if (!plans || plans.length === 0) {
    return (
      <p className="text-center text-zinc-500">
        No plans available. Please configure plans in the Clerk Dashboard.
      </p>
    );
  }

  // Sort plans by fee amount (free first, then ascending by price)
  const sortedPlans = [...plans].sort((a, b) => {
    if (!a.hasBaseFee) return -1;
    if (!b.hasBaseFee) return 1;
    return a.fee.amount - b.fee.amount;
  });

  // Determine which plan to highlight (starter or standard tier, or middle if not found)
  const highlightedIndex = sortedPlans.findIndex(p =>
    p.name.toLowerCase().includes('starter') || p.name.toLowerCase().includes('standard')
  );
  const highlighted = highlightedIndex >= 0 ? highlightedIndex : Math.floor(sortedPlans.length / 2);

  return (
    <div className={`grid gap-6 max-w-4xl mx-auto pt-4 ${
      sortedPlans.length === 1 ? 'md:grid-cols-1 max-w-md' :
      sortedPlans.length === 2 ? 'md:grid-cols-2 max-w-2xl' :
      sortedPlans.length >= 3 ? 'md:grid-cols-3' : ''
    }`}>
      {sortedPlans.map((plan, index) => (
        <PricingCard
          key={plan.id}
          name={plan.name}
          price={plan.hasBaseFee ? `${plan.fee.currencySymbol}${plan.fee.amountFormatted}` : '$0'}
          description={plan.description}
          features={plan.features.map(f => f.name)}
          highlighted={index === highlighted}
          descriptionHeight={descriptionHeight}
          featuresHeight={featuresHeight}
          descriptionRef={el => { descriptionRefs.current[index] = el; }}
          featuresRef={el => { featuresRefs.current[index] = el; }}
        />
      ))}
    </div>
  );
}

function PricingCard({
  name,
  price,
  description,
  features,
  highlighted = false,
  descriptionHeight,
  featuresHeight,
  descriptionRef,
  featuresRef,
}: {
  name: string;
  price: string;
  description: string | null;
  features: string[];
  highlighted?: boolean;
  descriptionHeight: number;
  featuresHeight: number;
  descriptionRef: (el: HTMLDivElement | null) => void;
  featuresRef: (el: HTMLUListElement | null) => void;
}) {
  return (
    <div className="relative">
      {/* Popular badge - attached label style */}
      {highlighted && (
        <div className="absolute -top-3 left-1/2 -translate-x-1/2 z-10">
          <span className="bg-indigo-500 text-white text-xs font-medium px-3 py-1 rounded-full shadow-sm">
            Popular
          </span>
        </div>
      )}
      <Card className={`bg-white/80 dark:bg-zinc-900/50 backdrop-blur-sm ${
        highlighted
          ? 'border-indigo-500 dark:border-indigo-400 ring-2 ring-indigo-500/20 dark:ring-indigo-400/20 shadow-lg shadow-indigo-500/10'
          : 'border-zinc-200/50 dark:border-zinc-800/50'
      }`}>
        <CardContent className="p-6">
          {/* Title */}
          <h3 className="text-2xl font-semibold leading-none tracking-tight">{name}</h3>

          {/* Description - fixed height matching tallest description */}
          <div
            ref={descriptionRef}
            className="mt-1.5"
            style={{ minHeight: descriptionHeight > 0 ? descriptionHeight : undefined }}
          >
            {description && (
              <p className="text-sm text-muted-foreground">{description}</p>
            )}
          </div>

          {/* Price */}
          <div className="mt-4 mb-6">
            <span className="text-2xl font-bold">{price}</span>
            <span className="text-zinc-500 text-sm">/month</span>
          </div>

          {/* Features - fixed height matching tallest features list */}
          <ul
            ref={featuresRef}
            className="space-y-3 text-sm"
            style={{ minHeight: featuresHeight > 0 ? featuresHeight : undefined }}
          >
            {features.map((feature, i) => (
              <li key={i} className="flex items-center gap-2">
                <svg className="h-4 w-4 flex-shrink-0 text-emerald-500" fill="none" viewBox="0 0 24 24" strokeWidth={2.5} stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                </svg>
                {feature}
              </li>
            ))}
          </ul>

          {/* Button */}
          <Link href="/sign-up" className="mt-6 block">
            <Button
              variant={highlighted ? 'default' : 'outline'}
              className="w-full"
            >
              Get Started
            </Button>
          </Link>
        </CardContent>
      </Card>
    </div>
  );
}
