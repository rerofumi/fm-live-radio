import {CSSProperties, useEffect, useMemo, useRef, useState} from 'react';
import './App.css';
import Visualizer, {LoudnessEnvelope} from './Visualizer';

import {GetNextItem, GetStatus, LoadConfig, PrefetchTalk, SaveConfig, SkipCurrent, UpdateStableAudio3Genre} from "../wailsjs/go/main/App";

type PlayableKind = "bgm" | "talk" | "silence";

type PlayableItem = {
  id: string;
  kind: PlayableKind;
  url?: string;
  loudnessUrl?: string;
  title: string;
  topicTitle?: string;
  durationHintMs?: number;
  source?: {
    provider?: string;
    genre?: string;
    prompt?: string;
    modelDir?: string;
  };
};

const SA3_GENRES: ReadonlyArray<string> = [
  "chill lo-fi",
  "smooth jazz",
  "minimal electronica",
  "ambient music",
];

const SA3_DEFAULT_GENRE = "chill lo-fi";

const SA3_GENRE_NEEDLE_ANGLES: Record<string, number> = {
  "chill lo-fi": -38,
  "smooth jazz": 38,
  "minimal electronica": -18,
  "ambient music": 18,
};

function normalizeSa3Genre(g: string | undefined | null): string {
  const v = (g ?? "").trim().toLowerCase();
  for (const allowed of SA3_GENRES) {
    if (v === allowed.toLowerCase()) return allowed;
  }
  return SA3_DEFAULT_GENRE;
}

type LampTone = "idle" | "active" | "ready" | "error";

function StatusLamp({label, tone, caption}: {label: string; tone: LampTone; caption: string}) {
  return (
    <div className={`statusLamp statusLamp-${tone}`} title={`${label}: ${caption}`}>
      <span className="lampBulb" />
      <span className="lampText">
        <span>{label}</span>
        <small>{caption}</small>
      </span>
    </div>
  );
}

function KnobControl({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
}) {
  const pct = Math.round(value * 100);
  const angle = -135 + value * 270;

  const handleMouseDown = (e: React.MouseEvent) => {
    if (e.button !== 0) return;
    e.preventDefault();

    const startY = e.clientY;
    const startValue = value;
    const dragScale = 150;

    const handleMouseMove = (moveEvent: MouseEvent) => {
      const deltaY = startY - moveEvent.clientY;
      const nextValue = Math.max(0, Math.min(1, startValue + deltaY / dragScale));
      onChange(nextValue);
    };

    const handleMouseUp = () => {
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleMouseUp);
    };

    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', handleMouseUp);
  };

  const handleTouchStart = (e: React.TouchEvent) => {
    if (e.touches.length === 0) return;
    const startY = e.touches[0].clientY;
    const startValue = value;
    const dragScale = 150;

    const handleTouchMove = (moveEvent: TouchEvent) => {
      if (moveEvent.touches.length === 0) return;
      const deltaY = startY - moveEvent.touches[0].clientY;
      const nextValue = Math.max(0, Math.min(1, startValue + deltaY / dragScale));
      onChange(nextValue);
    };

    const handleTouchEnd = () => {
      window.removeEventListener('touchmove', handleTouchMove);
      window.removeEventListener('touchend', handleTouchEnd);
    };

    window.addEventListener('touchmove', handleTouchMove, { passive: false });
    window.addEventListener('touchend', handleTouchEnd);
  };

  return (
    <div className="knobControl">
      <span className="knobLabel">{label}</span>
      <div
        className="knobDial"
        style={{"--knob-angle": `${angle}deg`} as CSSProperties}
        onMouseDown={handleMouseDown}
        onTouchStart={handleTouchStart}
        role="slider"
        aria-label={label}
        aria-valuenow={pct}
        aria-valuemin={0}
        aria-valuemax={100}
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === 'ArrowUp' || e.key === 'ArrowRight') {
            e.preventDefault();
            onChange(Math.min(1, value + 0.05));
          } else if (e.key === 'ArrowDown' || e.key === 'ArrowLeft') {
            e.preventDefault();
            onChange(Math.max(0, value - 0.05));
          }
        }}
      >
        <span className="knobTicks" />
        <span className="knobFace">
          <span className="knobPointerWrapper">
            <span className="knobPointer" />
          </span>
        </span>
      </div>
      <input
        type="range"
        min={0}
        max={1}
        step={0.01}
        value={value}
        onChange={(e) => onChange(parseFloat(e.target.value))}
        aria-label={`${label} volume`}
      />
      <span className="knobValue">{pct}%</span>
    </div>
  );
}

function GenreTuner({
  value,
  disabled,
  onSelect,
}: {
  value: string;
  disabled: boolean;
  onSelect: (genre: string) => void;
}) {
  const activeGenre = normalizeSa3Genre(value);
  const needleAngle = SA3_GENRE_NEEDLE_ANGLES[activeGenre] ?? SA3_GENRE_NEEDLE_ANGLES[SA3_DEFAULT_GENRE];

  return (
    <div className="genreTuner" style={{"--needle-angle": `${needleAngle}deg`} as CSSProperties}>
      <div className="tunerDial" aria-hidden="true">
        <div className="tunerScale">
          <span />
          <span />
          <span />
          <span />
          <span />
        </div>
        <div className="tunerNeedle" />
      </div>
      <div className="genreButtons" aria-label="SA3 Genre">
        {SA3_GENRES.map((genre) => (
          <button
            key={genre}
            type="button"
            className={`genreButton ${activeGenre === genre ? "isSelected" : ""}`}
            onClick={() => onSelect(genre)}
            disabled={disabled}
          >
            {genre}
          </button>
        ))}
      </div>
    </div>
  );
}

type AppConfig = {
  rssUrls: string[];

  bgmVolume: number;
  talkVolume: number;

  talk: {
    enabled: boolean;
    cycleBgmCount: number;
    targetDurationSec: number;
    silenceGapMinMs: number;
    silenceGapMaxMs: number;
  };
  llm: {
    enabled: boolean;
    baseUrl: string;
    apiKey: string;
    model: string;
  };
  localInference: {
    ortLibraryPath: string;
    maxWorkers: number;
    executionProvider: "cpu" | "cuda" | "auto";
    deviceId: number;
  };
  stableAudio3: {
    modelDir: string;
    outputDir: string;
    promptBase: string;
    genre: string;
    seconds: number;
    steps: number;
    seedMode: string;
    fixedSeed: number;
    cacheLimit: number;
  };
  irodori: {
    modelDir: string;
    narratorDir: string;
    refWav: string;
    seconds: number;
    numSteps: number;
    seedMode: string;
    fixedSeed: number;
    cfgText: number;
    cfgCaption: number;
    cfgSpeaker: number;
    durationScale: number;
  };
};

function App() {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const silenceTimerRef = useRef<number | null>(null);

  const [cfg, setCfg] = useState<AppConfig | null>(null);

  const [isPlaying, setIsPlaying] = useState(false);
  const isPlayingRef = useRef(false);
  const [current, setCurrent] = useState<PlayableItem | null>(null);
  const [bgmVolume, setBgmVolume] = useState(0.8);
  const [talkVolume, setTalkVolume] = useState(1.0);
  const [errorText, setErrorText] = useState<string>("");
  const [showSettings, setShowSettings] = useState(false);

  const [talkPrefetching, setTalkPrefetching] = useState(false);
  const [talkReady, setTalkReady] = useState(false);
  const [musicGenerating, setMusicGenerating] = useState(false);
  const [musicReady, setMusicReady] = useState(false);

  const [elapsedSec, setElapsedSec] = useState(0);
  const [durationSec, setDurationSec] = useState<number | null>(null);

  // Loudness envelope for the current item. Cleared immediately on item
  // change / skip and replaced only when the fetch resolves for the same item.
  const [loudness, setLoudness] = useState<LoudnessEnvelope | null>(null);

  const req = useMemo(() => ({}), []);

  useEffect(() => {
    (async () => {
      try {
        const loaded = (await LoadConfig()) as unknown as AppConfig;
        setCfg(loaded);
        setBgmVolume(typeof loaded.bgmVolume === 'number' ? loaded.bgmVolume : 0.8);
        setTalkVolume(typeof loaded.talkVolume === 'number' ? loaded.talkVolume : 1.0);
      } catch (e: any) {
        setErrorText(`設定読み込みに失敗しました: ${e?.message ?? String(e)}`);
      }
    })();
  }, []);

  function applyVolumeFor(kind: PlayableKind | undefined) {
    if (!audioRef.current) return;
    if (kind === 'talk') {
      audioRef.current.volume = talkVolume;
      return;
    }
    // default to bgm
    audioRef.current.volume = bgmVolume;
  }

  useEffect(() => {
    applyVolumeFor(current?.kind);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bgmVolume, talkVolume, current?.kind]);

  useEffect(() => {
    isPlayingRef.current = isPlaying;
  }, [isPlaying]);

  useEffect(() => {
    let timer: number | null = null;

    async function tick() {
      try {
        const st = await GetStatus();
        // @ts-ignore
        setTalkPrefetching(!!st?.talkPrefetching);
        // @ts-ignore
        setTalkReady(!!st?.talkReady);
        // @ts-ignore
        setMusicGenerating(!!st?.musicGenerating);
        // @ts-ignore
        setMusicReady(!!st?.musicReady);
        // @ts-ignore
        if (st?.localGenerationError) setErrorText(String(st.localGenerationError));
      } catch {
        // ignore
      }
    }

    // Poll while playing (and also when settings may trigger prefetch soon).
    if (isPlayingRef.current) {
      void tick();
      timer = window.setInterval(() => void tick(), 500);
    }

    return () => {
      if (timer != null) window.clearInterval(timer);
    };
  }, [isPlaying]);

  useEffect(() => {
    // cleanup timers on unmount
    return () => {
      if (silenceTimerRef.current != null) {
        window.clearTimeout(silenceTimerRef.current);
      }
    };
  }, []);

  function fmtTime(sec: number | null) {
    if (sec == null || !Number.isFinite(sec)) return '--:--';
    const s = Math.max(0, Math.floor(sec));
    const m = Math.floor(s / 60);
    const ss = String(s % 60).padStart(2, '0');
    return `${m}:${ss}`;
  }

  const progress = (() => {
    if (durationSec == null || durationSec <= 0) return 0;
    return Math.max(0, Math.min(1, elapsedSec / durationSec));
  })();

  useEffect(() => {
    let timer: number | null = null;

    function tick() {
      const a = audioRef.current;
      if (!a) return;

      if (current?.kind === 'silence') {
        // silence uses synthetic duration
        setElapsedSec((prev) => {
          // no-op; will be driven by separate timer below
          return prev;
        });
        return;
      }

      const d = Number.isFinite(a.duration) ? a.duration : null;
      const t = Number.isFinite(a.currentTime) ? a.currentTime : 0;
      setDurationSec(d);
      setElapsedSec(t);
    }

    if (isPlayingRef.current && current && current.kind !== 'silence') {
      tick();
      timer = window.setInterval(tick, 250);
    }

    return () => {
      if (timer != null) window.clearInterval(timer);
    };
  }, [current?.id, current?.kind, isPlaying]);

  // Fetch the loudness envelope once per item. We always reset to null first
  // so old envelopes never leak into a new item. Stale fetch responses (item
  // changed before fetch resolved) are ignored. Failures fall back silently
  // to the synthetic visualizer animation.
  useEffect(() => {
    setLoudness(null);

    const item = current;
    if (!item || item.kind === 'silence') {
      return;
    }
    const url = item.loudnessUrl;
    if (!url) {
      return;
    }

    const controller = new AbortController();
    const itemId = item.id;
    let cancelled = false;

    (async () => {
      let res: Response;
      try {
        res = await fetch(url, {signal: controller.signal});
      } catch {
        // network error — quietly stay on synthetic fallback
        return;
      }

      // 204 No Content: server has no envelope for this token (e.g. non-WAV).
      // Other non-2xx: expired token, network error response, etc.
      // Fall back silently in both cases.
      if (res.status === 204 || !res.ok) {
        return;
      }

      if (cancelled) return;

      let data: Partial<LoudnessEnvelope> | null = null;
      try {
        data = (await res.json()) as Partial<LoudnessEnvelope> | null;
      } catch {
        // empty or malformed body — fall back silently
        return;
      }

      if (!data || typeof data.windowMs !== 'number' || data.windowMs <= 0) return;
      if (!Array.isArray(data.rms) || data.rms.length === 0) return;
      setLoudness((prev) => {
        // Only accept if the item hasn't changed since this fetch was issued.
        if (currentIdRef.current !== itemId) return prev;
        return {
          windowMs: data!.windowMs!,
          sampleRate: typeof data!.sampleRate === 'number' ? data!.sampleRate : undefined,
          durationSec: typeof data!.durationSec === 'number' ? data!.durationSec : undefined,
          rms: data!.rms!.map((v) => (Number.isFinite(v) ? Math.max(0, Math.min(1, v)) : 0)),
          peak: Array.isArray(data!.peak)
            ? data!.peak.map((v) => (Number.isFinite(v) ? Math.max(0, Math.min(1, v)) : 0))
            : undefined,
        };
      });
    })();

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [current?.id, current?.kind, current?.loudnessUrl]);

  // Track the current item id so async fetch handlers can detect staleness
  // without re-subscribing.
  const currentIdRef = useRef<string | null>(null);
  useEffect(() => {
    currentIdRef.current = current?.id ?? null;
  }, [current?.id]);

  async function persistConfig(next: AppConfig) {
    setCfg(next);
    setBgmVolume(next.bgmVolume);
    setTalkVolume(next.talkVolume);
    await SaveConfig(next as any);
  }

  function stopPlayback() {
    isPlayingRef.current = false;
    setIsPlaying(false);
    if (silenceTimerRef.current != null) {
      window.clearTimeout(silenceTimerRef.current);
      silenceTimerRef.current = null;
    }
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current.src = "";
    }
    setLoudness(null);
  }

  async function playLoopNext() {
    if (!isPlayingRef.current) return;

    let item: PlayableItem;
    try {
      item = (await GetNextItem(req as any)) as unknown as PlayableItem;
    } catch (e: any) {
      setErrorText(`次アイテム取得に失敗: ${e?.message ?? String(e)}`);
      stopPlayback();
      return;
    }

    setCurrent(item);

    // reset progress UI
    setElapsedSec(0);
    setDurationSec(null);

    if (item.kind === "silence") {
      const ms = Math.max(0, item.durationHintMs ?? 0);
      const start = performance.now();
      setDurationSec(ms / 1000);

      const update = () => {
        const elapsed = (performance.now() - start) / 1000;
        setElapsedSec(Math.min(elapsed, ms / 1000));
      };

      const interval = window.setInterval(update, 100);

      silenceTimerRef.current = window.setTimeout(() => {
        window.clearInterval(interval);
        silenceTimerRef.current = null;
        playLoopNext();
      }, ms);
      return;
    }

    if (!audioRef.current || !item.url) {
      setErrorText("再生URLがありません");
      stopPlayback();
      return;
    }

    try {
      applyVolumeFor(item.kind);
      audioRef.current.src = item.url;
      await audioRef.current.play();

      // initialize duration/current time once metadata is ready
      const a = audioRef.current;
      if (a) {
        const d = Number.isFinite(a.duration) ? a.duration : null;
        setDurationSec(d);
        setElapsedSec(Number.isFinite(a.currentTime) ? a.currentTime : 0);
      }

      // best-effort prefetch hint
      PrefetchTalk();
    } catch (e: any) {
      setErrorText(`再生開始に失敗: ${e?.message ?? String(e)}`);
      stopPlayback();
    }
  }

  async function onPlayPause() {
    setErrorText("");
    if (!cfg) return;

    if (isPlayingRef.current) {
      stopPlayback();
      return;
    }

    isPlayingRef.current = true;
    setIsPlaying(true);
    // kick off loop immediately (avoid stale state closure)
    void playLoopNext();
  }

  async function onSkip() {
    setErrorText("");
    if (!isPlayingRef.current) return;

    if (silenceTimerRef.current != null) {
      window.clearTimeout(silenceTimerRef.current);
      silenceTimerRef.current = null;
    }

    try {
      const skipReq = {
        // Send current kind so backend can apply correct skip semantics.
        currentKind: (current?.kind ?? "bgm") as PlayableKind,
      };

      const item = (await SkipCurrent(skipReq as any)) as unknown as PlayableItem;
      setCurrent(item);
      if (item.kind === "silence") {
        const ms = Math.max(0, item.durationHintMs ?? 0);
        silenceTimerRef.current = window.setTimeout(() => {
          silenceTimerRef.current = null;
          playLoopNext();
        }, ms);
        return;
      }
      if (audioRef.current && item.url) {
        audioRef.current.pause();
        applyVolumeFor(item.kind);
        audioRef.current.src = item.url;
        await audioRef.current.play();
      }
    } catch (e: any) {
      setErrorText(`スキップに失敗: ${e?.message ?? String(e)}`);
      stopPlayback();
    }
  }

  const nowTitle = current
    ? (current.kind === 'talk' ? (current.topicTitle ?? current.title) : current.title)
    : '未再生';

  const nowSub = (() => {
    if (!current) return '再生を開始してください';
    if (current.kind === 'talk') {
      const parts = ['ニューストーク'];
      if (current.source?.provider) parts.push(current.source.provider);
      return parts.join(' · ');
    }
    if (current.kind === 'bgm') {
      const parts = ['BGM'];
      if (current.source?.provider) parts.push(current.source.provider);
      if (current.source?.genre) parts.push(current.source.genre);
      return parts.join(' · ');
    }
    return '間（無音）';
  })();

  const currentLevel = current?.kind === 'talk' ? talkVolume : bgmVolume;
  const selectedGenre = normalizeSa3Genre(cfg?.stableAudio3?.genre);
  const errorLampTone: LampTone = errorText ? "error" : "idle";

  function updateBgmVolume(value: number) {
    setBgmVolume(value);
    if (cfg) {
      void persistConfig({...cfg, bgmVolume: value});
    }
  }

  function updateTalkVolume(value: number) {
    setTalkVolume(value);
    if (cfg) {
      void persistConfig({...cfg, talkVolume: value});
    }
  }

  function updateGenre(genre: string) {
    const next = normalizeSa3Genre(genre);
    if (!cfg) return;
    const nextCfg: AppConfig = {
      ...cfg,
      stableAudio3: {...cfg.stableAudio3, genre: next},
    };
    setCfg(nextCfg);
    // Use the dedicated binding so currently playing / prefetched BGM is not
    // interrupted. The backend also persists the new value to config.json.
    UpdateStableAudio3Genre(next).catch((e: any) => {
      setErrorText(`ジャンル保存に失敗: ${e?.message ?? String(e)}`);
    });
  }

  return (
    <div className="app">
      <div className="radioCabinet">
        <header className="radioHeader">
          <div className="brand">
            <div className="brandMark" aria-hidden="true" />
            <div className="brandTitle">
              <h1>fm-live-radio</h1>
              <span>AI ローカルラジオ</span>
            </div>
          </div>
          <button className="settingsButton" onClick={() => setShowSettings(true)} aria-label="Settings" title="Settings">
            <span aria-hidden="true">⚙</span>
          </button>
        </header>

        {errorText ? (
          <div className="toast">{errorText}</div>
        ) : null}

        <main className="radioDeck">
          <section className="controlPanel" aria-label="radio controls">
            <div className="waveDisplay">
              <div className="waveHeader">
                <span className="waveSub" title={nowSub}>{nowSub}</span>
                <span className="timeBadge">{fmtTime(elapsedSec)} / {fmtTime(durationSec)}</span>
              </div>
              <Visualizer
                playing={isPlaying}
                kind={current?.kind}
                level={currentLevel}
                audio={audioRef.current}
                loudness={loudness}
              />
              <div className="progressRow" aria-label="playback progress">
                <div className="progressBar">
                  <div className="progressFill" style={{width: `${Math.round(progress * 100)}%`}} />
                </div>
              </div>
            </div>

            <div className="controlLeft">
              <div className="transportPanel">
                <button
                  className={`playBtn ${isPlaying ? 'isPlaying' : ''}`}
                  onClick={onPlayPause}
                  disabled={!cfg}
                  aria-label={isPlaying ? 'Pause' : 'Play'}
                  title={isPlaying ? 'Pause' : 'Play'}
                >
                  {isPlaying ? <span className="icoPause"><i /><i /></span> : <span className="icoPlay" />}
                </button>
                <button className="skipButton" onClick={onSkip} disabled={!isPlaying}>
                  Skip
                </button>
              </div>

              <div className="lampPanel" aria-label="generation status">
                <StatusLamp
                  label="Talk"
                  tone={talkReady ? "ready" : (talkPrefetching ? "active" : "idle")}
                  caption={talkReady ? "ready" : (talkPrefetching ? "making" : "idle")}
                />
                <StatusLamp
                  label="Music"
                  tone={musicReady ? "ready" : (musicGenerating ? "active" : "idle")}
                  caption={musicReady ? "ready" : (musicGenerating ? "making" : "idle")}
                />
                <StatusLamp
                  label="Local"
                  tone={errorLampTone}
                  caption={errorText ? "error" : "normal"}
                />
              </div>
            </div>

            <div className="mixerPanel">
              <KnobControl label="BGM" value={bgmVolume} onChange={updateBgmVolume} />
              <KnobControl label="Talk" value={talkVolume} onChange={updateTalkVolume} />
            </div>
          </section>

          <section className="tuningPanel" aria-label="tuning meter">
            <div className="tuningMeter">
              <div className="meterTop">
                <span className={`onair meterOnair ${isPlaying ? 'isLive' : ''}`}>
                  <span className="onairDot" />
                  {isPlaying ? 'ON AIR' : 'OFF AIR'}
                </span>
                <span className="kindPill">{current?.kind ?? 'idle'}</span>
              </div>
              <div className="meterGlass">
                <GenreTuner value={selectedGenre} disabled={!cfg} onSelect={updateGenre} />
              </div>
            </div>
          </section>
        </main>

        <footer className="nameplate">
          <span>Now playing</span>
          <strong title={nowTitle}>{nowTitle}</strong>
        </footer>

        <audio
          ref={audioRef}
          onLoadedMetadata={() => {
            const a = audioRef.current;
            if (!a) return;
            setDurationSec(Number.isFinite(a.duration) ? a.duration : null);
          }}
          onTimeUpdate={() => {
            const a = audioRef.current;
            if (!a) return;
            setElapsedSec(Number.isFinite(a.currentTime) ? a.currentTime : 0);
          }}
          onEnded={() => {
            if (!isPlayingRef.current) return;
            void playLoopNext();
          }}
          onError={() => {
            if (!isPlayingRef.current) return;
            void playLoopNext();
          }}
        />

        {showSettings && cfg ? (
          <div className="modalOverlay" onMouseDown={() => setShowSettings(false)}>
            <div className="modal" onMouseDown={(e) => e.stopPropagation()}>
              <div className="modalHeader">
                <h2>Settings</h2>
                <button className="btn" onClick={() => setShowSettings(false)}>Close</button>
              </div>

              <h3 className="settings-section-title">アプリ機能</h3>
              <div className="modalGrid">
                <label>曲数 (BGM→Talk)</label>
                <input
                  type="number"
                  min={1}
                  max={20}
                  step={1}
                  value={cfg.talk?.cycleBgmCount ?? 3}
                  onChange={(e) => {
                    const n = parseInt(e.target.value, 10);
                    setCfg({...cfg, talk: {...cfg.talk, cycleBgmCount: Number.isFinite(n) && n > 0 ? n : 3}});
                  }}
                />

                <label>Silence Gap Min (ms)</label>
                <input
                  type="number"
                  min={0}
                  step={100}
                  value={cfg.talk?.silenceGapMinMs ?? 1000}
                  onChange={(e) => {
                    const n = parseInt(e.target.value, 10);
                    setCfg({...cfg, talk: {...cfg.talk, silenceGapMinMs: Number.isFinite(n) && n >= 0 ? n : 1000}});
                  }}
                />

                <label>Silence Gap Max (ms)</label>
                <input
                  type="number"
                  min={0}
                  step={100}
                  value={cfg.talk?.silenceGapMaxMs ?? 3000}
                  onChange={(e) => {
                    const n = parseInt(e.target.value, 10);
                    setCfg({...cfg, talk: {...cfg.talk, silenceGapMaxMs: Number.isFinite(n) && n >= 0 ? n : 3000}});
                  }}
                />

                <label>BGM Volume</label>
                <input
                  type="range"
                  min={0}
                  max={1}
                  step={0.01}
                  value={cfg.bgmVolume}
                  onChange={(e) => setCfg({...cfg, bgmVolume: parseFloat(e.target.value)})}
                />

                <label>Talk Volume</label>
                <input
                  type="range"
                  min={0}
                  max={1}
                  step={0.01}
                  value={cfg.talkVolume}
                  onChange={(e) => setCfg({...cfg, talkVolume: parseFloat(e.target.value)})}
                />

                <label>RSS URLs (1行1URL)</label>
                <textarea
                  rows={4}
                  value={(cfg.rssUrls ?? []).join("\n")}
                  onChange={(e) => setCfg({...cfg, rssUrls: e.target.value.split("\n").map(s => s.trim()).filter(Boolean)})}
                  placeholder="https://example.com/rss"
                />

                <label>LLM Base URL</label>
                <input
                  value={cfg.llm?.baseUrl ?? ""}
                  onChange={(e) => setCfg({...cfg, llm: {...cfg.llm, baseUrl: e.target.value}})}
                  placeholder="http://localhost:11434/v1"
                />

                <label>LLM API Key</label>
                <input
                  value={cfg.llm?.apiKey ?? ""}
                  onChange={(e) => setCfg({...cfg, llm: {...cfg.llm, apiKey: e.target.value}})}
                  placeholder="(optional)"
                />

                <label>LLM Model</label>
                <input
                  value={cfg.llm?.model ?? ""}
                  onChange={(e) => setCfg({...cfg, llm: {...cfg.llm, model: e.target.value}})}
                  placeholder="gpt-4o-mini"
                />
              </div>

              <details className="settings-details">
                <summary>生成設定</summary>
                <div className="modalGrid" style={{ marginTop: '10px' }}>
                  <label>ORT DLL Path</label>
                  <input
                    value={cfg.localInference?.ortLibraryPath ?? ""}
                    onChange={(e) => setCfg({...cfg, localInference: {...cfg.localInference, ortLibraryPath: e.target.value}})}
                    placeholder="C:/path/to/onnxruntime.dll"
                  />

                  <label>Local Inference EP</label>
                  <select
                    value={cfg.localInference?.executionProvider ?? "auto"}
                    onChange={(e) => setCfg({...cfg, localInference: {...cfg.localInference, executionProvider: e.target.value as AppConfig["localInference"]["executionProvider"]}})}
                  >
                    <option value="auto">Auto</option>
                    <option value="cuda">CUDA</option>
                    <option value="cpu">CPU</option>
                  </select>

                  <label>Local Inference Device ID</label>
                  <input
                    type="number"
                    min={0}
                    step={1}
                    value={cfg.localInference?.deviceId ?? 0}
                    onChange={(e) => {
                      const n = parseInt(e.target.value, 10);
                      setCfg({...cfg, localInference: {...cfg.localInference, deviceId: Number.isFinite(n) && n >= 0 ? n : 0}});
                    }}
                  />

                  <label>SA3 Model Dir</label>
                  <input
                    value={cfg.stableAudio3?.modelDir ?? ""}
                    onChange={(e) => setCfg({...cfg, stableAudio3: {...cfg.stableAudio3, modelDir: e.target.value}})}
                    placeholder="E:/.../model/sa3-sm-music"
                  />

                  <label>SA3 Output Dir</label>
                  <input
                    value={cfg.stableAudio3?.outputDir ?? ""}
                    onChange={(e) => setCfg({...cfg, stableAudio3: {...cfg.stableAudio3, outputDir: e.target.value}})}
                    placeholder="E:/.../generate_music"
                  />

                  <label>SA3 Genre</label>
                  <select
                    id="sa3GenreSettings"
                    className="genreSelect"
                    value={normalizeSa3Genre(cfg.stableAudio3?.genre)}
                    onChange={(e) => setCfg({...cfg, stableAudio3: {...cfg.stableAudio3, genre: normalizeSa3Genre(e.target.value)}})}
                  >
                    {SA3_GENRES.map((g) => (
                      <option key={g} value={g}>{g}</option>
                    ))}
                  </select>

                  <label>SA3 Prompt Base</label>
                  <input
                    value={cfg.stableAudio3?.promptBase ?? ""}
                    onChange={(e) => setCfg({...cfg, stableAudio3: {...cfg.stableAudio3, promptBase: e.target.value}})}
                  />

                  <label>SA3 Seconds</label>
                  <input
                    type="number"
                    value={cfg.stableAudio3?.seconds ?? 30}
                    onChange={(e) => setCfg({...cfg, stableAudio3: {...cfg.stableAudio3, seconds: parseFloat(e.target.value)}})}
                  />

                  <label>SA3 Steps</label>
                  <input
                    type="number"
                    value={cfg.stableAudio3?.steps ?? 8}
                    onChange={(e) => setCfg({...cfg, stableAudio3: {...cfg.stableAudio3, steps: parseInt(e.target.value, 10)}})}
                  />

                  <label>SA3 Seed Mode</label>
                  <select
                    value={cfg.stableAudio3?.seedMode ?? "random"}
                    onChange={(e) => setCfg({...cfg, stableAudio3: {...cfg.stableAudio3, seedMode: e.target.value}})}
                  >
                    <option value="random">Random</option>
                    <option value="fixed">Fixed</option>
                    <option value="sequential">Sequential</option>
                  </select>

                  <label>SA3 Fixed Seed</label>
                  <input
                    type="number"
                    value={cfg.stableAudio3?.fixedSeed ?? 0}
                    onChange={(e) => setCfg({...cfg, stableAudio3: {...cfg.stableAudio3, fixedSeed: parseInt(e.target.value, 10)}})}
                  />

                  <label>SA3 Cache Limit</label>
                  <input
                    type="number"
                    value={cfg.stableAudio3?.cacheLimit ?? 20}
                    onChange={(e) => setCfg({...cfg, stableAudio3: {...cfg.stableAudio3, cacheLimit: parseInt(e.target.value, 10)}})}
                  />

                  <label>Irodori Model Dir</label>
                  <input
                    value={cfg.irodori?.modelDir ?? ""}
                    onChange={(e) => setCfg({...cfg, irodori: {...cfg.irodori, modelDir: e.target.value}})}
                    placeholder="E:/.../model/irodori-v3"
                  />

                  <label>Irodori Narrator Dir</label>
                  <input
                    value={cfg.irodori?.narratorDir ?? ""}
                    onChange={(e) => setCfg({...cfg, irodori: {...cfg.irodori, narratorDir: e.target.value}})}
                    placeholder="E:/.../narrator"
                  />

                  <label>Irodori Ref WAV</label>
                  <input
                    value={cfg.irodori?.refWav ?? ""}
                    onChange={(e) => setCfg({...cfg, irodori: {...cfg.irodori, refWav: e.target.value}})}
                    placeholder="(optional)"
                  />

                  <label>Irodori Steps</label>
                  <input
                    type="number"
                    value={cfg.irodori?.numSteps ?? 40}
                    onChange={(e) => setCfg({...cfg, irodori: {...cfg.irodori, numSteps: parseInt(e.target.value, 10)}})}
                  />

                  <label>Irodori Seed Mode</label>
                  <select
                    value={cfg.irodori?.seedMode ?? "random"}
                    onChange={(e) => setCfg({...cfg, irodori: {...cfg.irodori, seedMode: e.target.value}})}
                  >
                    <option value="random">Random</option>
                    <option value="fixed">Fixed</option>
                    <option value="sequential">Sequential</option>
                  </select>

                  <label>Irodori Fixed Seed</label>
                  <input
                    type="number"
                    value={cfg.irodori?.fixedSeed ?? 0}
                    onChange={(e) => setCfg({...cfg, irodori: {...cfg.irodori, fixedSeed: parseInt(e.target.value, 10)}})}
                  />

                  <label>Irodori CFG Text</label>
                  <input
                    type="number"
                    step="0.1"
                    value={cfg.irodori?.cfgText ?? 3}
                    onChange={(e) => setCfg({...cfg, irodori: {...cfg.irodori, cfgText: parseFloat(e.target.value)}})}
                  />

                  <label>Irodori CFG Caption</label>
                  <input
                    type="number"
                    step="0.1"
                    value={cfg.irodori?.cfgCaption ?? 3}
                    onChange={(e) => setCfg({...cfg, irodori: {...cfg.irodori, cfgCaption: parseFloat(e.target.value)}})}
                  />

                  <label>Irodori CFG Speaker</label>
                  <input
                    type="number"
                    step="0.1"
                    value={cfg.irodori?.cfgSpeaker ?? 5}
                    onChange={(e) => setCfg({...cfg, irodori: {...cfg.irodori, cfgSpeaker: parseFloat(e.target.value)}})}
                  />

                  <label>Irodori Duration Scale</label>
                  <input
                    type="number"
                    step="0.1"
                    value={cfg.irodori?.durationScale ?? 1}
                    onChange={(e) => setCfg({...cfg, irodori: {...cfg.irodori, durationScale: parseFloat(e.target.value)}})}
                  />
                </div>
              </details>

              <div className="modalFooter">
                <button className="btn" onClick={() => setShowSettings(false)}>Cancel</button>
                <button
                  className="btn btnPrimary"
                  onClick={async () => {
                    try {
                      await persistConfig(cfg);
                      setShowSettings(false);
                    } catch (e: any) {
                      setErrorText(`保存に失敗: ${e?.message ?? String(e)}`);
                    }
                  }}
                >
                  Save
                </button>
              </div>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
}

export default App;
