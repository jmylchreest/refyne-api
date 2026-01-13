'use client';

import { useEffect, useRef, useState } from 'react';
import { useTheme } from 'next-themes';

interface GridNode {
  x: number;
  y: number;
  baseX: number;
  baseY: number;
  vx: number;
  vy: number;
  pulse: number;
  pulseSpeed: number;
}

export function AnimatedGrid() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const nodesRef = useRef<GridNode[]>([]);
  const mouseRef = useRef({ x: -1000, y: -1000 });
  const animationRef = useRef<number | null>(null);
  const { resolvedTheme } = useTheme();
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !mounted || !resolvedTheme) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const resizeCanvas = () => {
      const dpr = window.devicePixelRatio || 1;
      // Use window dimensions for full coverage
      const width = window.innerWidth;
      const height = window.innerHeight;
      canvas.width = width * dpr;
      canvas.height = height * dpr;
      ctx.scale(dpr, dpr);
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      initNodes(width, height);
    };

    const initNodes = (width: number, height: number) => {
      const nodes: GridNode[] = [];
      const spacing = 60;
      const cols = Math.ceil(width / spacing) + 2;
      const rows = Math.ceil(height / spacing) + 2;

      for (let i = 0; i < cols; i++) {
        for (let j = 0; j < rows; j++) {
          const x = i * spacing - spacing;
          const y = j * spacing - spacing;
          nodes.push({
            x,
            y,
            baseX: x,
            baseY: y,
            vx: 0,
            vy: 0,
            pulse: Math.random() * Math.PI * 2,
            pulseSpeed: 0.01 + Math.random() * 0.02,
          });
        }
      }
      nodesRef.current = nodes;
    };

    const handleMouseMove = (e: MouseEvent) => {
      const rect = canvas.getBoundingClientRect();
      mouseRef.current = {
        x: e.clientX - rect.left,
        y: e.clientY - rect.top,
      };
    };

    const handleMouseLeave = () => {
      mouseRef.current = { x: -1000, y: -1000 };
    };

    const animate = () => {
      if (!ctx || !canvas) return;

      const width = window.innerWidth;
      const height = window.innerHeight;
      ctx.clearRect(0, 0, width, height);

      const isDark = resolvedTheme === 'dark';
      const lineColor = isDark ? 'rgba(255, 255, 255, 0.025)' : 'rgba(0, 0, 0, 0.025)';
      const nodeColor = isDark ? 'rgba(255, 255, 255, 0.05)' : 'rgba(0, 0, 0, 0.04)';
      const pulseColor = isDark ? 'rgba(99, 102, 241, 0.2)' : 'rgba(99, 102, 241, 0.15)';

      const nodes = nodesRef.current;
      const mouse = mouseRef.current;
      const time = Date.now() * 0.001;

      // Update node positions with wave effect and mouse interaction
      nodes.forEach((node) => {
        // Ambient wave motion
        const waveX = Math.sin(time * 0.5 + node.baseY * 0.01) * 3;
        const waveY = Math.cos(time * 0.3 + node.baseX * 0.01) * 3;

        // Mouse repulsion
        const dx = node.baseX - mouse.x;
        const dy = node.baseY - mouse.y;
        const dist = Math.sqrt(dx * dx + dy * dy);
        const maxDist = 150;

        let pushX = 0;
        let pushY = 0;
        if (dist < maxDist && dist > 0) {
          const force = (1 - dist / maxDist) * 20;
          pushX = (dx / dist) * force;
          pushY = (dy / dist) * force;
        }

        // Spring back to base position
        const targetX = node.baseX + waveX + pushX;
        const targetY = node.baseY + waveY + pushY;
        node.vx += (targetX - node.x) * 0.1;
        node.vy += (targetY - node.y) * 0.1;
        node.vx *= 0.9;
        node.vy *= 0.9;
        node.x += node.vx;
        node.y += node.vy;

        // Update pulse
        node.pulse += node.pulseSpeed;
      });

      // Draw connections
      ctx.strokeStyle = lineColor;
      ctx.lineWidth = 1;

      const spacing = 60;
      const rows = Math.ceil(height / spacing) + 2;

      nodes.forEach((node, i) => {
        // Connect to right neighbor (nodes are arranged column by column)
        if ((i + 1) % rows !== 0) {
          const rightNeighbor = nodes[i + 1];
          if (rightNeighbor) {
            ctx.beginPath();
            ctx.moveTo(node.x, node.y);
            ctx.lineTo(rightNeighbor.x, rightNeighbor.y);
            ctx.stroke();
          }
        }

        // Connect to bottom neighbor
        const bottomNeighbor = nodes[i + rows];
        if (bottomNeighbor) {
          ctx.beginPath();
          ctx.moveTo(node.x, node.y);
          ctx.lineTo(bottomNeighbor.x, bottomNeighbor.y);
          ctx.stroke();
        }
      });

      // Draw nodes with pulse effect
      nodes.forEach((node) => {
        const pulseIntensity = (Math.sin(node.pulse) + 1) * 0.5;
        const size = 1.5 + pulseIntensity * 1;

        // Draw pulse glow for some nodes
        if (pulseIntensity > 0.8) {
          ctx.beginPath();
          ctx.arc(node.x, node.y, size + 4, 0, Math.PI * 2);
          ctx.fillStyle = pulseColor;
          ctx.globalAlpha = (pulseIntensity - 0.8) * 2;
          ctx.fill();
          ctx.globalAlpha = 1;
        }

        // Draw node
        ctx.beginPath();
        ctx.arc(node.x, node.y, size, 0, Math.PI * 2);
        ctx.fillStyle = nodeColor;
        ctx.fill();
      });

      animationRef.current = requestAnimationFrame(animate);
    };

    resizeCanvas();
    window.addEventListener('resize', resizeCanvas);
    canvas.addEventListener('mousemove', handleMouseMove);
    canvas.addEventListener('mouseleave', handleMouseLeave);
    animate();

    return () => {
      window.removeEventListener('resize', resizeCanvas);
      canvas.removeEventListener('mousemove', handleMouseMove);
      canvas.removeEventListener('mouseleave', handleMouseLeave);
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current);
      }
    };
  }, [resolvedTheme, mounted]);

  // Always render canvas, animation will start when theme is resolved
  return (
    <canvas
      ref={canvasRef}
      className="absolute inset-0 -z-10 pointer-events-auto"
      style={{ width: '100%', height: '100%' }}
    />
  );
}
