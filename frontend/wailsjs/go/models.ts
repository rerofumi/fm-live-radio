export namespace domain {
	
	export class TTSConfig {
	    enabled: boolean;
	    model: string;
	    voice: string;
	
	    static createFrom(source: any = {}) {
	        return new TTSConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.model = source["model"];
	        this.voice = source["voice"];
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
	    bgmRootPath: string;
	    selectedGenre: string;
	    rssUrls: string[];
	    geminiApiKey: string;
	    bgmVolume: number;
	    talkVolume: number;
	    talk: TalkConfig;
	    llm: LLMConfig;
	    tts: TTSConfig;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bgmRootPath = source["bgmRootPath"];
	        this.selectedGenre = source["selectedGenre"];
	        this.rssUrls = source["rssUrls"];
	        this.geminiApiKey = source["geminiApiKey"];
	        this.bgmVolume = source["bgmVolume"];
	        this.talkVolume = source["talkVolume"];
	        this.talk = this.convertValues(source["talk"], TalkConfig);
	        this.llm = this.convertValues(source["llm"], LLMConfig);
	        this.tts = this.convertValues(source["tts"], TTSConfig);
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
	
	    static createFrom(source: any = {}) {
	        return new AppStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.talkPrefetching = source["talkPrefetching"];
	        this.talkReady = source["talkReady"];
	    }
	}
	
	export class NextItemRequest {
	    selectedGenre: string;
	
	    static createFrom(source: any = {}) {
	        return new NextItemRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.selectedGenre = source["selectedGenre"];
	    }
	}
	export class PlayableSource {
	    genre?: string;
	    filePath?: string;
	    rssUrl?: string;
	    articleUrl?: string;
	
	    static createFrom(source: any = {}) {
	        return new PlayableSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.genre = source["genre"];
	        this.filePath = source["filePath"];
	        this.rssUrl = source["rssUrl"];
	        this.articleUrl = source["articleUrl"];
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
	
	

}

