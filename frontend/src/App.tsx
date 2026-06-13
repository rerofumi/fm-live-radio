import {useEffect, useMemo, useRef, useState} from 'react';
import './App.css';
import Visualizer from './Visualizer';

import {GetNextItem, GetStatus, LoadConfig, PrefetchTalk, SaveConfig, SkipCurrent, UpdateStableAudio3Genre} from "../wailsjs/go/main/App";

type PlayableKind = "bgm" | "talk" | "silence";

type PlayableItem = {
  id: string;
  kind: PlayableKind;
  url?: string;
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

function normalizeSa3Genre(g: string | undefined | null): string {
  const v = (g ?? "").trim().toLowerCase();
  for (const allowed of SA3_GENRES) {
    if (v === allowed.toLowerCase()) return allowed;
  }
  return SA3_DEFAULT_GENRE;
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

  return (
    <div className="app">
      <div className="shell">
        <header className="topbar">
          <div className="brand">
            <div className="brandMark" />
            <div className="brandTitle">
              <h1>fm-live-radio</h1>
              <span>AI ローカルラジオ</span>
            </div>
          </div>
          <div className="statusGroup">
            <div
              className={`chip ${talkReady ? 'isReady' : (talkPrefetching ? 'isActive' : '')}`}
              title={talkReady ? 'Talk ready' : (talkPrefetching ? 'Generating talk...' : 'Talk idle')}
            >
              <span className="dot" />
              <span>Talk</span>
            </div>
            <div
              className={`chip ${musicReady ? 'isReady' : (musicGenerating ? 'isActive' : '')}`}
              title={musicReady ? 'Music ready' : (musicGenerating ? 'Generating music...' : 'Music idle')}
            >
              <span className="dot" />
              <span>Music</span>
            </div>
            <button className="btn btnGhost" onClick={() => setShowSettings(true)}>
              Settings
            </button>
          </div>
        </header>

        {errorText ? (
          <div className="toast">{errorText}</div>
        ) : null}

        <section className="stage">
          <div className="stageTop">
            <div className={`onair ${isPlaying ? 'isLive' : ''}`}>
              <span className="onairDot" />
              {isPlaying ? 'ON AIR' : 'OFF AIR'}
            </div>
            <div className="kindPill">{current?.kind ?? 'idle'}</div>
          </div>

          <Visualizer playing={isPlaying} kind={current?.kind} level={currentLevel} />

          <div className="stageInfo">
            <h2 className="nowTitle" title={nowTitle}>{nowTitle}</h2>
            <div className="nowSub" title={nowSub}>{nowSub}</div>

            <div className="progressRow" aria-label="playback progress">
              <div className="progressBar">
                <div className="progressFill" style={{width: `${Math.round(progress * 100)}%`}} />
              </div>
              <div className="timeText">
                {fmtTime(elapsedSec)} / {fmtTime(durationSec)}
              </div>
            </div>
          </div>
        </section>

        <section className="console">
          <div className="transport">
            <button
              className={`playBtn ${isPlaying ? 'isPlaying' : ''}`}
              onClick={onPlayPause}
              disabled={!cfg}
              aria-label={isPlaying ? 'Pause' : 'Play'}
              title={isPlaying ? 'Pause' : 'Play'}
            >
              {isPlaying ? <span className="icoPause"><i /><i /></span> : <span className="icoPlay" />}
            </button>
            <button className="btn" onClick={onSkip} disabled={!isPlaying}>
              Skip
            </button>

          </div>

          <div className="mixer">
            <div className="range">
              <div className="rangeLabel">BGM</div>
              <input
                type="range"
                min={0}
                max={1}
                step={0.01}
                value={bgmVolume}
                onChange={(e) => {
                  const v = parseFloat(e.target.value);
                  setBgmVolume(v);
                  if (cfg) {
                    void persistConfig({...cfg, bgmVolume: v});
                  }
                }}
              />
              <div className="kv">{Math.round(bgmVolume * 100)}%</div>
            </div>

            <div className="range">
              <div className="rangeLabel">Talk</div>
              <input
                type="range"
                min={0}
                max={1}
                step={0.01}
                value={talkVolume}
                onChange={(e) => {
                  const v = parseFloat(e.target.value);
                  setTalkVolume(v);
                  if (cfg) {
                    void persistConfig({...cfg, talkVolume: v});
                  }
                }}
              />
              <div className="kv">{Math.round(talkVolume * 100)}%</div>
            </div>
          </div>

          <div className="genre">
            <label htmlFor="sa3GenreConsole">Genre</label>
            <select
              id="sa3GenreConsole"
              className="genreSelect"
              value={normalizeSa3Genre(cfg?.stableAudio3?.genre)}
              onChange={(e) => {
                const next = normalizeSa3Genre(e.target.value);
                if (!cfg) return;
                const nextCfg: AppConfig = {
                  ...cfg,
                  stableAudio3: {...cfg.stableAudio3, genre: next},
                };
                setCfg(nextCfg);
                // Use the dedicated binding so currently playing / prefetched
                // BGM is not interrupted (FR-10). The backend also persists
                // the new value to config.json.
                UpdateStableAudio3Genre(next).catch((e: any) => {
                  setErrorText(`ジャンル保存に失敗: ${e?.message ?? String(e)}`);
                });
              }}
            >
              {SA3_GENRES.map((g) => (
                <option key={g} value={g}>{g}</option>
              ))}
            </select>
          </div>

          <div className="hint">
            Talk はBGM再生中に先読み生成されます（設定により変動）。音は止まらず流れ続けます。
          </div>
        </section>

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
