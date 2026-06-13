export namespace domain {
	
	export class IrodoriConfig {
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
	
	    static createFrom(source: any = {}) {
	        return new IrodoriConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.modelDir = source["modelDir"];
	        this.narratorDir = source["narratorDir"];
	        this.refWav = source["refWav"];
	        this.seconds = source["seconds"];
	        this.numSteps = source["numSteps"];
	        this.seedMode = source["seedMode"];
	        this.fixedSeed = source["fixedSeed"];
	        this.cfgText = source["cfgText"];
	        this.cfgCaption = source["cfgCaption"];
	        this.cfgSpeaker = source["cfgSpeaker"];
	        this.durationScale = source["durationScale"];
	    }
	}
	export class StableAudio3Config {
	    modelDir: string;
	    outputDir: string;
	    promptBase: string;
	    genre: string;
	    seconds: number;
	    steps: number;
	    seedMode: string;
	    fixedSeed: number;
	    cacheLimit: number;
	
	    static createFrom(source: any = {}) {
	        return new StableAudio3Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.modelDir = source["modelDir"];
	        this.outputDir = source["outputDir"];
	        this.promptBase = source["promptBase"];
	        this.genre = source["genre"];
	        this.seconds = source["seconds"];
	        this.steps = source["steps"];
	        this.seedMode = source["seedMode"];
	        this.fixedSeed = source["fixedSeed"];
	        this.cacheLimit = source["cacheLimit"];
	    }
	}
	export class LocalInferenceConfig {
	    ortLibraryPath: string;
	    maxWorkers: number;
	    executionProvider: string;
	    deviceId: number;
	
	    static createFrom(source: any = {}) {
	        return new LocalInferenceConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ortLibraryPath = source["ortLibraryPath"];
	        this.maxWorkers = source["maxWorkers"];
	        this.executionProvider = source["executionProvider"];
	        this.deviceId = source["deviceId"];
	    }
	}
	export class LLMConfig {
	    enabled: boolean;
	    baseUrl: string;
	    apiKey: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.baseUrl = source["baseUrl"];
	        this.apiKey = source["apiKey"];
	        this.model = source["model"];
	    }
	}
	export class TalkConfig {
	    enabled: boolean;
	    cycleBgmCount: number;
	    targetDurationSec: number;
	    silenceGapMinMs: number;
	    silenceGapMaxMs: number;
	
	    static createFrom(source: any = {}) {
	        return new TalkConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.cycleBgmCount = source["cycleBgmCount"];
	        this.targetDurationSec = source["targetDurationSec"];
	        this.silenceGapMinMs = source["silenceGapMinMs"];
	        this.silenceGapMaxMs = source["silenceGapMaxMs"];
	    }
	}
	export class AppConfig {
	    rssUrls: string[];
	    bgmVolume: number;
	    talkVolume: number;
	    talk: TalkConfig;
	    llm: LLMConfig;
	    localInference: LocalInferenceConfig;
	    stableAudio3: StableAudio3Config;
	    irodori: IrodoriConfig;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rssUrls = source["rssUrls"];
	        this.bgmVolume = source["bgmVolume"];
	        this.talkVolume = source["talkVolume"];
	        this.talk = this.convertValues(source["talk"], TalkConfig);
	        this.llm = this.convertValues(source["llm"], LLMConfig);
	        this.localInference = this.convertValues(source["localInference"], LocalInferenceConfig);
	        this.stableAudio3 = this.convertValues(source["stableAudio3"], StableAudio3Config);
	        this.irodori = this.convertValues(source["irodori"], IrodoriConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AppStatus {
	    talkPrefetching: boolean;
	    talkReady: boolean;
	    musicGenerating: boolean;
	    musicReady: boolean;
	    localGenerationError?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.talkPrefetching = source["talkPrefetching"];
	        this.talkReady = source["talkReady"];
	        this.musicGenerating = source["musicGenerating"];
	        this.musicReady = source["musicReady"];
	        this.localGenerationError = source["localGenerationError"];
	    }
	}
	
	
	
	export class NextItemRequest {
	
	
	    static createFrom(source: any = {}) {
	        return new NextItemRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class PlayableSource {
	    genre?: string;
	    filePath?: string;
	    rssUrl?: string;
	    articleUrl?: string;
	    provider?: string;
	    prompt?: string;
	    seed?: number;
	    modelDir?: string;
	
	    static createFrom(source: any = {}) {
	        return new PlayableSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.genre = source["genre"];
	        this.filePath = source["filePath"];
	        this.rssUrl = source["rssUrl"];
	        this.articleUrl = source["articleUrl"];
	        this.provider = source["provider"];
	        this.prompt = source["prompt"];
	        this.seed = source["seed"];
	        this.modelDir = source["modelDir"];
	    }
	}
	export class PlayableItem {
	    id: string;
	    kind: string;
	    url?: string;
	    mime?: string;
	    title: string;
	    artist?: string;
	    topicTitle?: string;
	    durationHintMs?: number;
	    source?: PlayableSource;
	
	    static createFrom(source: any = {}) {
	        return new PlayableItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.url = source["url"];
	        this.mime = source["mime"];
	        this.title = source["title"];
	        this.artist = source["artist"];
	        this.topicTitle = source["topicTitle"];
	        this.durationHintMs = source["durationHintMs"];
	        this.source = this.convertValues(source["source"], PlayableSource);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SkipRequest {
	    currentKind: string;
	
	    static createFrom(source: any = {}) {
	        return new SkipRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentKind = source["currentKind"];
	    }
	}
	

}

