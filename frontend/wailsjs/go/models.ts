export namespace apperr {
	
	export class AppError {
	    code: string;
	    message: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.message = source["message"];
	        this.detail = source["detail"];
	    }
	}

}

export namespace jsontool {
	
	export class CompareRequest {
	    json1: string;
	    json2: string;
	
	    static createFrom(source: any = {}) {
	        return new CompareRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.json1 = source["json1"];
	        this.json2 = source["json2"];
	    }
	}
	export class ErrorPos {
	    line: number;
	    column: number;
	
	    static createFrom(source: any = {}) {
	        return new ErrorPos(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = source["line"];
	        this.column = source["column"];
	    }
	}
	export class Difference {
	    type: string;
	    path: string;
	    oldValue: string;
	    newValue: string;
	
	    static createFrom(source: any = {}) {
	        return new Difference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.path = source["path"];
	        this.oldValue = source["oldValue"];
	        this.newValue = source["newValue"];
	    }
	}
	export class CompareResponse {
	    identical: boolean;
	    differences: Difference[];
	    error: string;
	    errorPos: ErrorPos;
	
	    static createFrom(source: any = {}) {
	        return new CompareResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.identical = source["identical"];
	        this.differences = this.convertValues(source["differences"], Difference);
	        this.error = source["error"];
	        this.errorPos = this.convertValues(source["errorPos"], ErrorPos);
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
	
	
	export class FormatRequest {
	    json: string;
	    indent: number;
	    compact: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FormatRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.json = source["json"];
	        this.indent = source["indent"];
	        this.compact = source["compact"];
	    }
	}
	export class FormatResponse {
	    formatted: string;
	    error: string;
	    errorPos: ErrorPos;
	
	    static createFrom(source: any = {}) {
	        return new FormatResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.formatted = source["formatted"];
	        this.error = source["error"];
	        this.errorPos = this.convertValues(source["errorPos"], ErrorPos);
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
	export class ValidateRequest {
	    json: string;
	
	    static createFrom(source: any = {}) {
	        return new ValidateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.json = source["json"];
	    }
	}
	export class ValidateResponse {
	    valid: boolean;
	    error: string;
	    errorPos: ErrorPos;
	
	    static createFrom(source: any = {}) {
	        return new ValidateResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.valid = source["valid"];
	        this.error = source["error"];
	        this.errorPos = this.convertValues(source["errorPos"], ErrorPos);
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

export namespace media {
	
	export class Thumb {
	    path: string;
	    dataUrl: string;
	    atSec: number;
	    width: number;
	
	    static createFrom(source: any = {}) {
	        return new Thumb(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.dataUrl = source["dataUrl"];
	        this.atSec = source["atSec"];
	        this.width = source["width"];
	    }
	}

}

export namespace store {
	
	export class StreamInfo {
	    index: number;
	    type: string;
	    codec: string;
	    profile?: string;
	    width?: number;
	    height?: number;
	    pixFmt?: string;
	    fps?: number;
	    bitrate?: number;
	    duration?: number;
	    rotation?: number;
	    sampleRate?: number;
	    channels?: number;
	    channelLayout?: string;
	    language?: string;
	    attachedPic?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StreamInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.type = source["type"];
	        this.codec = source["codec"];
	        this.profile = source["profile"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.pixFmt = source["pixFmt"];
	        this.fps = source["fps"];
	        this.bitrate = source["bitrate"];
	        this.duration = source["duration"];
	        this.rotation = source["rotation"];
	        this.sampleRate = source["sampleRate"];
	        this.channels = source["channels"];
	        this.channelLayout = source["channelLayout"];
	        this.language = source["language"];
	        this.attachedPic = source["attachedPic"];
	    }
	}
	export class MediaInfo {
	    id: string;
	    path: string;
	    name: string;
	    size: number;
	    duration: number;
	    width: number;
	    height: number;
	    videoCodec: string;
	    audioCodec: string;
	    bitrate: number;
	    thumbUrl: string;
	    container?: string;
	    fps?: number;
	    rotation?: number;
	    sampleRate?: number;
	    channels?: number;
	    hasVideo?: boolean;
	    hasAudio?: boolean;
	    streams?: StreamInfo[];
	    error?: apperr.AppError;
	    probedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new MediaInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.duration = source["duration"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.videoCodec = source["videoCodec"];
	        this.audioCodec = source["audioCodec"];
	        this.bitrate = source["bitrate"];
	        this.thumbUrl = source["thumbUrl"];
	        this.container = source["container"];
	        this.fps = source["fps"];
	        this.rotation = source["rotation"];
	        this.sampleRate = source["sampleRate"];
	        this.channels = source["channels"];
	        this.hasVideo = source["hasVideo"];
	        this.hasAudio = source["hasAudio"];
	        this.streams = this.convertValues(source["streams"], StreamInfo);
	        this.error = this.convertValues(source["error"], apperr.AppError);
	        this.probedAt = source["probedAt"];
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
	
	export class Task {
	    id: string;
	    type: string;
	    status: string;
	    title: string;
	    inputPaths: string[];
	    outputPath: string;
	    progress: number;
	    speed: string;
	    etaSec: number;
	    params: string;
	    version: number;
	    error?: apperr.AppError;
	    createdAt: number;
	    startedAt: number;
	    finishedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.status = source["status"];
	        this.title = source["title"];
	        this.inputPaths = source["inputPaths"];
	        this.outputPath = source["outputPath"];
	        this.progress = source["progress"];
	        this.speed = source["speed"];
	        this.etaSec = source["etaSec"];
	        this.params = source["params"];
	        this.version = source["version"];
	        this.error = this.convertValues(source["error"], apperr.AppError);
	        this.createdAt = source["createdAt"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
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
	export class TaskFilter {
	    types: string[];
	    statuses: string[];
	    limit: number;
	    offset: number;
	
	    static createFrom(source: any = {}) {
	        return new TaskFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.types = source["types"];
	        this.statuses = source["statuses"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	    }
	}
	export class TaskPage {
	    items: Task[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new TaskPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], Task);
	        this.total = source["total"];
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

export namespace system {
	
	export class FFmpegStatus {
	    state: string;
	    path: string;
	    version: string;
	    source: string;
	    taskId?: string;
	    ffprobeMissing: boolean;
	    error?: apperr.AppError;
	
	    static createFrom(source: any = {}) {
	        return new FFmpegStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.path = source["path"];
	        this.version = source["version"];
	        this.source = source["source"];
	        this.taskId = source["taskId"];
	        this.ffprobeMissing = source["ffprobeMissing"];
	        this.error = this.convertValues(source["error"], apperr.AppError);
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
	export class InstallOptions {
	    platform: string;
	    supported: boolean;
	    mirrors: string[];
	
	    static createFrom(source: any = {}) {
	        return new InstallOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.platform = source["platform"];
	        this.supported = source["supported"];
	        this.mirrors = source["mirrors"];
	    }
	}
	export class Settings {
	    ffmpegPath: string;
	    ffmpegPromptDismissed: boolean;
	    defaultOutputDir: string;
	    maxConcurrent: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ffmpegPath = source["ffmpegPath"];
	        this.ffmpegPromptDismissed = source["ffmpegPromptDismissed"];
	        this.defaultOutputDir = source["defaultOutputDir"];
	        this.maxConcurrent = source["maxConcurrent"];
	    }
	}

}

