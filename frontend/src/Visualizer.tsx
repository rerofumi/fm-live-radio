import {useEffect, useRef} from 'react';

type Kind = 'bgm' | 'talk' | 'silence';

export type LoudnessEnvelope = {
  windowMs: number;
  sampleRate?: number;
  durationSec?: number;
  rms: number[];
  peak?: number[];
};

type VisualizerProps = {
  /** True while audio (or a silence gap) is on air. */
  playing: boolean;
  /** Current item kind; changes the wave hue. */
  kind?: Kind;
  /** 0..1 target volume for the current kind; modulates amplitude. */
  level?: number;
  /** Current <audio> element so we can sample currentTime in the loop. */
  audio?: HTMLAudioElement | null;
  /** Precomputed RMS envelope for the current item, or null when unavailable. */
  loudness?: LoudnessEnvelope | null;
};

// Hue per state. The wave always flows, but its color reflects what is on air.
const HUE: Record<string, number> = {
  idle: 236, // calm indigo
  bgm: 248, // indigo / violet
  talk: 22, // warm amber
  silence: 205, // cool blue-grey
};

type WaveTarget = {amp: number; energy: number; flow: number; hue: number};

function baseTargetFor(playing: boolean, kind: Kind | undefined, level: number): WaveTarget {
  const hue = HUE[kind ?? 'idle'] ?? HUE.idle;
  const lvl = Math.max(0, Math.min(1, level));
  if (!playing) {
    // Idle / paused: never fully still — sound is "always flowing".
    return {amp: 0.12, energy: 0.22, flow: 0.45, hue: HUE.idle};
  }
  if (kind === 'silence') {
    return {amp: 0.09, energy: 0.3, flow: 0.5, hue};
  }
  if (kind === 'talk') {
    return {amp: 0.32 + 0.22 * lvl, energy: 0.85, flow: 1.1, hue};
  }
  // bgm (default)
  return {amp: 0.44 + 0.3 * lvl, energy: 1, flow: 1, hue};
}

function isValidEnvelope(env: LoudnessEnvelope | null | undefined): env is LoudnessEnvelope {
  if (!env) return false;
  if (typeof env.windowMs !== 'number' || env.windowMs <= 0) return false;
  if (!Array.isArray(env.rms) || env.rms.length === 0) return false;
  return true;
}

function sampleLoudness(env: LoudnessEnvelope, timeSec: number): number {
  const idx = Math.floor((timeSec * 1000) / env.windowMs);
  if (idx < 0) return env.rms[0] ?? 0;
  if (idx >= env.rms.length) return env.rms[env.rms.length - 1] ?? 0;
  const v = env.rms[idx];
  if (!Number.isFinite(v)) return 0;
  if (v < 0) return 0;
  if (v > 1) return 1;
  return v;
}

const LAYERS = [
  {freq: 1.0, speed: 0.6, weight: 1.0, alpha: 0.3},
  {freq: 1.7, speed: -0.95, weight: 0.62, alpha: 0.22},
  {freq: 2.6, speed: 1.35, weight: 0.4, alpha: 0.15},
];

export default function Visualizer({playing, kind, level = 0.8, audio = null, loudness = null}: VisualizerProps) {
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  // Keep latest props without restarting the animation loop.
  const propsRef = useRef<VisualizerProps>({playing, kind, level, audio, loudness});
  propsRef.current = {playing, kind, level, audio, loudness};

  // Lets the reduced-motion effect trigger a repaint on prop changes.
  const repaintStaticRef = useRef<() => void>(() => {});

  useEffect(() => {
    const wrap = wrapRef.current;
    const canvas = canvasRef.current;
    if (!wrap || !canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const media = window.matchMedia('(prefers-reduced-motion: reduce)');
    let reduced = media.matches;

    let width = 1;
    let height = 1;

    // Smoothed wave state (interpolated toward the current target).
    const cur: WaveTarget = {...baseTargetFor(playing, kind, level ?? 0.8)};
    let phase = 0;

    const computeTarget = (): WaveTarget => {
      const p = propsRef.current;
      const tgt = baseTargetFor(p.playing, p.kind, p.level ?? 0.8);
      const env = isValidEnvelope(p.loudness) ? p.loudness! : null;
      const a = p.audio;
      // Only mix loudness when actively playing real audio (not silence / paused).
      if (
        env &&
        a &&
        p.playing &&
        p.kind !== 'silence' &&
        !a.paused &&
        Number.isFinite(a.currentTime)
      ) {
        const lvl = Math.max(0, Math.min(1, p.level ?? 0.8));
        const raw = sampleLoudness(env, a.currentTime);
        const audible = raw * lvl;
        // Mix into amp/energy while keeping the kind/level baseline.
        tgt.amp = Math.min(1, tgt.amp + audible * 0.55);
        tgt.energy = Math.min(1.6, tgt.energy + audible * 0.35);
      }
      return tgt;
    };

    const paint = (timeSec: number) => {
      ctx.clearRect(0, 0, width, height);
      const mid = height * 0.5;
      const breathe = 0.85 + 0.15 * Math.sin(timeSec * 1.2);
      const baseAmp = cur.amp * height * 0.42 * breathe;

      for (const layer of LAYERS) {
        ctx.beginPath();
        for (let x = 0; x <= width; x += 2) {
          const nx = x / width;
          const y =
            mid +
            Math.sin(nx * Math.PI * 2 * layer.freq + phase * layer.speed * 6) * baseAmp * layer.weight +
            Math.sin(nx * Math.PI * 2 * layer.freq * 0.5 - phase * layer.speed * 3) * baseAmp * layer.weight * 0.4;
          if (x === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        }
        ctx.lineTo(width, height);
        ctx.lineTo(0, height);
        ctx.closePath();

        const alpha = layer.alpha * (0.45 + 0.55 * cur.energy);
        const grad = ctx.createLinearGradient(0, mid - baseAmp, 0, height);
        grad.addColorStop(0, `hsla(${cur.hue}, 85%, 62%, ${alpha})`);
        grad.addColorStop(1, `hsla(${cur.hue + 36}, 85%, 60%, 0)`);
        ctx.fillStyle = grad;
        ctx.fill();
      }

      // Bright "signal" line riding on top of the layered body.
      ctx.beginPath();
      for (let x = 0; x <= width; x += 2) {
        const nx = x / width;
        const y = mid + Math.sin(nx * Math.PI * 2 * LAYERS[0].freq + phase * LAYERS[0].speed * 6) * baseAmp;
        if (x === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      }
      ctx.lineWidth = 2;
      ctx.strokeStyle = `hsla(${cur.hue}, 90%, 56%, ${0.45 + 0.55 * cur.energy})`;
      ctx.shadowColor = `hsla(${cur.hue}, 90%, 60%, ${0.4 * cur.energy})`;
      ctx.shadowBlur = 14 * cur.energy;
      ctx.stroke();
      ctx.shadowBlur = 0;
    };

    const resize = () => {
      const dpr = Math.max(1, Math.min(2, window.devicePixelRatio || 1));
      const rect = wrap.getBoundingClientRect();
      width = Math.max(1, Math.floor(rect.width));
      height = Math.max(1, Math.floor(rect.height));
      canvas.width = Math.floor(width * dpr);
      canvas.height = Math.floor(height * dpr);
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      if (reduced) paint(0);
    };

    resize();
    const ro = new ResizeObserver(resize);
    ro.observe(wrap);

    let raf = 0;
    let last = performance.now();

    const loop = (now: number) => {
      const dt = Math.min(0.05, (now - last) / 1000);
      last = now;

      const tgt = computeTarget();
      // Frame-rate independent smoothing toward the target.
      const k = 1 - Math.pow(0.0008, dt);
      cur.amp += (tgt.amp - cur.amp) * k;
      cur.energy += (tgt.energy - cur.energy) * k;
      cur.hue += (tgt.hue - cur.hue) * k;
      cur.flow += (tgt.flow - cur.flow) * k;

      phase += cur.flow * dt;
      paint(now / 1000);
      raf = requestAnimationFrame(loop);
    };

    const renderStatic = () => {
      const tgt = computeTarget();
      cur.amp = tgt.amp;
      cur.energy = tgt.energy;
      cur.hue = tgt.hue;
      cur.flow = 0;
      phase = 0.25;
      paint(0);
    };
    repaintStaticRef.current = renderStatic;

    const start = () => {
      cancelAnimationFrame(raf);
      if (reduced) {
        renderStatic();
      } else {
        last = performance.now();
        raf = requestAnimationFrame(loop);
      }
    };

    const onMediaChange = () => {
      reduced = media.matches;
      start();
    };
    media.addEventListener('change', onMediaChange);

    start();

    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
      media.removeEventListener('change', onMediaChange);
      repaintStaticRef.current = () => {};
    };
  }, []);

  // Reduced-motion users get a fresh static frame when the state changes.
  useEffect(() => {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      repaintStaticRef.current();
    }
  }, [playing, kind, level, audio, loudness]);

  return (
    <div className="viz" ref={wrapRef} aria-hidden="true">
      <canvas ref={canvasRef} />
    </div>
  );
}
