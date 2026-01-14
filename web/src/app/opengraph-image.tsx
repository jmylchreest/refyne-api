import { ImageResponse } from 'next/og';
import { OG_FEATURES, OG_TAGLINE, OG_URL } from '@/lib/og-constants';

export const runtime = 'edge';
export const alt = `Refyne - ${OG_TAGLINE}`;
export const size = {
  width: 1200,
  height: 630,
};
export const contentType = 'image/png';

export default async function Image() {
  // Load local TTF fonts for the OG image (WOFF2 not supported by @vercel/og)
  const victorMonoItalic = await fetch(
    new URL('../../public/fonts/VictorMono-BoldItalic.ttf', import.meta.url)
  ).then((res) => res.arrayBuffer());

  const firaCode = await fetch(
    new URL('../../public/fonts/FiraCode-Medium.ttf', import.meta.url)
  ).then((res) => res.arrayBuffer());

  return new ImageResponse(
    (
      <div
        style={{
          height: '100%',
          width: '100%',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          background: 'linear-gradient(135deg, #09090b 0%, #18181b 50%, #27272a 100%)',
          fontFamily: 'system-ui, sans-serif',
        }}
      >
        {/* Grid pattern overlay */}
        <div
          style={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundImage: 'linear-gradient(rgba(99, 102, 241, 0.03) 1px, transparent 1px), linear-gradient(90deg, rgba(99, 102, 241, 0.03) 1px, transparent 1px)',
            backgroundSize: '50px 50px',
          }}
        />

        {/* Gradient orb */}
        <div
          style={{
            position: 'absolute',
            top: '50%',
            left: '50%',
            transform: 'translate(-50%, -50%)',
            width: '600px',
            height: '600px',
            background: 'radial-gradient(circle, rgba(99, 102, 241, 0.15) 0%, transparent 70%)',
            borderRadius: '50%',
          }}
        />

        {/* Main content */}
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            position: 'relative',
          }}
        >
          {/* Logo text */}
          <div
            style={{
              fontSize: 96,
              fontFamily: 'Victor Mono',
              fontWeight: 700,
              fontStyle: 'italic',
              background: 'linear-gradient(135deg, #f4f4f5 0%, #a1a1aa 100%)',
              backgroundClip: 'text',
              color: 'transparent',
              letterSpacing: '-0.02em',
              marginBottom: 20,
            }}
          >
            Refyne
          </div>

          {/* Tagline */}
          <div
            style={{
              fontSize: 32,
              fontFamily: 'Fira Code',
              color: '#71717a',
              fontWeight: 500,
              marginBottom: 40,
            }}
          >
            {OG_TAGLINE}
          </div>

          {/* Feature badges */}
          <div
            style={{
              display: 'flex',
              gap: 16,
            }}
          >
            {OG_FEATURES.map((feature) => (
              <div
                key={feature}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  padding: '12px 24px',
                  background: 'rgba(99, 102, 241, 0.1)',
                  border: '1px solid rgba(99, 102, 241, 0.2)',
                  borderRadius: 999,
                  fontSize: 18,
                  color: '#818cf8',
                }}
              >
                {feature}
              </div>
            ))}
          </div>
        </div>

        {/* URL at bottom */}
        <div
          style={{
            position: 'absolute',
            bottom: 40,
            fontSize: 24,
            fontFamily: 'Fira Code',
            color: '#52525b',
          }}
        >
          {OG_URL}
        </div>
      </div>
    ),
    {
      ...size,
      fonts: [
        {
          name: 'Victor Mono',
          data: victorMonoItalic,
          style: 'italic',
          weight: 700,
        },
        {
          name: 'Fira Code',
          data: firaCode,
          style: 'normal',
          weight: 500,
        },
      ],
    }
  );
}
