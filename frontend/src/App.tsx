import {useEffect, useMemo, useRef, useState} from 'react';
import './App.css';

import {GetNextItem, LoadConfig, PrefetchTalk, SaveConfig, ScanGenres, SkipCurrent} from "../wailsjs/go/main/App";

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
    // cleanup timers on unmount
    return () => {
      if (silenceTimerRef.current != null) {
        window.clearTimeout(silenceTimerRef.current);
      }
    };
  }, []);

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

    if (item.kind === "silence") {
      const ms = Math.max(0, item.durationHintMs ?? 0);
      silenceTimerRef.current = window.setTimeout(() => {
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
      const item = (await SkipCurrent(req as any)) as unknown as PlayableItem;
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

  return (
    <div style={{padding: 16, display: 'flex', flexDirection: 'column', gap: 12}}>
      <h2 style={{margin: 0}}>fm-live-radio</h2>

      {errorText ? (
        <div style={{background: '#3b1d1d', padding: 8, borderRadius: 6}}>{errorText}</div>
      ) : null}

      <div style={{display: 'flex', gap: 8, alignItems: 'center'}}>
        <label>Genre:</label>
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
          <option value="">(未選択)</option>
          {genres.map((g) => (
            <option key={g} value={g}>{g}</option>
          ))}
        </select>

        <button onClick={onPlayPause} disabled={!cfg}>
          {isPlaying ? 'Pause' : 'Play'}
        </button>
        <button onClick={onSkip} disabled={!isPlaying}>
          Skip
        </button>
        <button onClick={() => setShowSettings(true)}>
          Settings
        </button>

        <label style={{marginLeft: 12}}>BGM Vol</label>
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
        <label>Talk Vol</label>
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
      </div>

      <div style={{background: '#112', padding: 12, borderRadius: 8}}>
        <div>Now: {current ? `${current.kind} - ${current.kind === 'talk' ? (current.topicTitle ?? current.title) : current.title}` : '(none)'}</div>
      </div>

      <audio
        ref={audioRef}
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
        <div style={{position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)'}}>
          <div style={{maxWidth: 720, margin: '5vh auto', background: '#0f172a', padding: 16, borderRadius: 12}}>
            <h3 style={{marginTop: 0}}>Settings</h3>

            <div style={{display: 'grid', gridTemplateColumns: '180px 1fr', gap: 8, alignItems: 'center'}}>
              <label>BGM Root Path</label>
              <input
                value={cfg.bgmRootPath}
                onChange={(e) => setCfg({...cfg, bgmRootPath: e.target.value})}
              />

              <label>Gemini API Key</label>
              <input
                value={cfg.geminiApiKey}
                onChange={(e) => setCfg({...cfg, geminiApiKey: e.target.value})}
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

              <label>TTS Model</label>
              <input
                value={cfg.tts?.model ?? ""}
                onChange={(e) => setCfg({...cfg, tts: {...cfg.tts, model: e.target.value}})}
              />

              <label>TTS Voice</label>
              <input
                value={cfg.tts?.voice ?? ""}
                onChange={(e) => setCfg({...cfg, tts: {...cfg.tts, voice: e.target.value}})}
              />

              <label>RSS URLs (1行1URL)</label>
              <textarea
                rows={6}
                value={(cfg.rssUrls ?? []).join("\n")}
                onChange={(e) => setCfg({...cfg, rssUrls: e.target.value.split("\n").map(s => s.trim()).filter(Boolean)})}
              />

              <label>LLM Base URL</label>
              <input
                value={cfg.llm?.baseUrl ?? ""}
                onChange={(e) => setCfg({...cfg, llm: {...cfg.llm, baseUrl: e.target.value}})}
              />

              <label>LLM API Key</label>
              <input
                value={cfg.llm?.apiKey ?? ""}
                onChange={(e) => setCfg({...cfg, llm: {...cfg.llm, apiKey: e.target.value}})}
              />

              <label>LLM Model</label>
              <input
                value={cfg.llm?.model ?? ""}
                onChange={(e) => setCfg({...cfg, llm: {...cfg.llm, model: e.target.value}})}
              />
            </div>

            <div style={{display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 12}}>
              <button onClick={() => setShowSettings(false)}>Cancel</button>
              <button
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
  );
}

export default App;
