import {useEffect, useMemo, useRef, useState} from 'react';
import './App.css';

import {GetNextItem, GetStatus, LoadConfig, PrefetchTalk, SaveConfig, ScanGenres, SkipCurrent} from "../wailsjs/go/main/App";

type PlayableKind = "bgm" | "talk" | "silence";

type PlayableItem = {
  id: string;
  kind: PlayableKind;
  url?: string;
  title: string;
  topicTitle?: string;
  durationHintMs?: number;
};

type AppConfig = {
  bgmRootPath: string;
  selectedGenre: string;
  rssUrls: string[];
  geminiApiKey: string;

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
  tts: {
    enabled: boolean;
    model: string;
    voice: string;
  };
};

function App() {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const silenceTimerRef = useRef<number | null>(null);

  const [cfg, setCfg] = useState<AppConfig | null>(null);
  const [genres, setGenres] = useState<string[]>([]);
  const [selectedGenre, setSelectedGenre] = useState<string>("");

  const [isPlaying, setIsPlaying] = useState(false);
  const isPlayingRef = useRef(false);
  const [current, setCurrent] = useState<PlayableItem | null>(null);
  const [bgmVolume, setBgmVolume] = useState(0.8);
  const [talkVolume, setTalkVolume] = useState(1.0);
  const [errorText, setErrorText] = useState<string>("");
  const [showSettings, setShowSettings] = useState(false);

  const [talkPrefetching, setTalkPrefetching] = useState(false);
  const [talkReady, setTalkReady] = useState(false);

  const [elapsedSec, setElapsedSec] = useState(0);
  const [durationSec, setDurationSec] = useState<number | null>(null);

  const req = useMemo(() => ({ selectedGenre }), [selectedGenre]);

  useEffect(() => {
    (async () => {
      try {
        const loaded = (await LoadConfig()) as unknown as AppConfig;
        setCfg(loaded);
        setSelectedGenre(loaded.selectedGenre ?? "");
        setBgmVolume(typeof loaded.bgmVolume === 'number' ? loaded.bgmVolume : 0.8);
        setTalkVolume(typeof loaded.talkVolume === 'number' ? loaded.talkVolume : 1.0);
      } catch (e: any) {
        setErrorText(`設定読み込みに失敗しました: ${e?.message ?? String(e)}`);
      }
    })();
  }, []);

  useEffect(() => {
    if (!cfg?.bgmRootPath) {
      setGenres([]);
      return;
    }
    (async () => {
      try {
        const g = await ScanGenres();
        setGenres(g);
      } catch (e: any) {
        setGenres([]);
      }
    })();
  }, [cfg?.bgmRootPath]);

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
    setSelectedGenre(next.selectedGenre);
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
        selectedGenre,
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

  const nowSub = current
    ? (current.kind === 'talk' ? 'ニューストーク' : current.kind === 'bgm' ? 'BGM' : '間（無音）')
    : '再生を開始してください';

  return (
    <div className="app">
      <div className="shell">
        <header className="header">
          <div className="brand">
            <div className="brandMark" />
            <div className="brandTitle">
              <h1>fm-live-radio</h1>
              <span>AIローカルラジオ</span>
            </div>
          </div>
          <div className="toolbar">
            <div className="pilot" title={talkReady ? 'Talk ready' : (talkPrefetching ? 'Generating talk...' : 'Talk idle')}>
              <span className={`lamp ${talkReady ? 'lampReady' : (talkPrefetching ? 'lampActive' : '')}`} />
              <span className="pilotText">Talk</span>
            </div>
            <button className="btn" onClick={() => setShowSettings(true)}>
              Settings
            </button>
          </div>
        </header>

        {errorText ? (
          <div className="toast">{errorText}</div>
        ) : null}

        <section className="card">
          <div className="cardTitle">Controls</div>

          <div className="controlsRow" style={{marginBottom: 12}}>
            <div className="field">
              <label>Genre</label>
              <select
                value={selectedGenre}
                onChange={(e) => {
                  const g = e.target.value;
                  setSelectedGenre(g);
                  if (cfg) {
                    void persistConfig({...cfg, selectedGenre: g});
                  }
                }}
                disabled={!cfg}
              >
                <option value="">未選択</option>
                {genres.map((g) => (
                  <option key={g} value={g}>{g}</option>
                ))}
              </select>
            </div>

            <button className={`btn ${isPlaying ? 'btnDanger' : 'btnPrimary'}`} onClick={onPlayPause} disabled={!cfg}>
              {isPlaying ? 'Pause' : 'Play'}
            </button>
            <button className="btn" onClick={onSkip} disabled={!isPlaying}>
              Skip
            </button>
          </div>

          <div className="range" style={{marginBottom: 10}}>
            <div className="small">BGM Vol</div>
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
            <div className="small">Talk Vol</div>
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

          <div className="small" style={{marginTop: 12}}>
            Talk生成はBGM再生中に先読みされます（設定により変動）。
          </div>
        </section>

        <section className="card">
          <div className="cardTitle">Now Playing</div>
          <div className="now">
            <div className="art" />
            <div className="nowText">
              <div className="kindPill">
                {current?.kind ?? 'idle'}
              </div>
              <h3 className="nowTitle" title={nowTitle}>{nowTitle}</h3>
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

              <div className="modalGrid">
                <label>BGM Root Path</label>
                <input
                  value={cfg.bgmRootPath}
                  onChange={(e) => setCfg({...cfg, bgmRootPath: e.target.value})}
                  placeholder="E:/Music/BGM"
                />

                <label>Selected Genre</label>
                <input
                  value={cfg.selectedGenre}
                  onChange={(e) => setCfg({...cfg, selectedGenre: e.target.value})}
                  placeholder="Lo-Fi"
                />

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

                <label>Gemini API Key</label>
                <input
                  value={cfg.geminiApiKey}
                  onChange={(e) => setCfg({...cfg, geminiApiKey: e.target.value})}
                  placeholder="AIza..."
                />

                <label>TTS Model</label>
                <input
                  value={cfg.tts?.model ?? ""}
                  onChange={(e) => setCfg({...cfg, tts: {...cfg.tts, model: e.target.value}})}
                  placeholder="gemini-2.5-flash-preview-tts"
                />

                <label>TTS Voice</label>
                <input
                  value={cfg.tts?.voice ?? ""}
                  onChange={(e) => setCfg({...cfg, tts: {...cfg.tts, voice: e.target.value}})}
                  placeholder="Kore"
                />

                <label>RSS URLs (1行1URL)</label>
                <textarea
                  rows={6}
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
