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

export namespace convert {
	
	export class AddSourceResult {
	    path: string;
	    source?: store.ConvertSource;
	    existed: boolean;
	    error?: apperr.AppError;
	
	    static createFrom(source: any = {}) {
	        return new AddSourceResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.source = this.convertValues(source["source"], store.ConvertSource);
	        this.existed = source["existed"];
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
	export class ConvertSearchFilter {
	    keyword: string;
	    limit: number;
	    offset: number;
	    recordLimit: number;
	    status?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConvertSearchFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.keyword = source["keyword"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	        this.recordLimit = source["recordLimit"];
	        this.status = source["status"];
	    }
	}
	export class ConvertSourceEntry {
	    source: store.ConvertSource;
	    records: store.Task[];
	    recordCount: number;
	    nameMatched?: boolean;
	    matchedTaskIds?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ConvertSourceEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = this.convertValues(source["source"], store.ConvertSource);
	        this.records = this.convertValues(source["records"], store.Task);
	        this.recordCount = source["recordCount"];
	        this.nameMatched = source["nameMatched"];
	        this.matchedTaskIds = source["matchedTaskIds"];
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
	export class ConvertSourceFilter {
	    limit: number;
	    offset: number;
	    recordLimit: number;
	    status?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConvertSourceFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	        this.recordLimit = source["recordLimit"];
	        this.status = source["status"];
	    }
	}
	export class ConvertSourcePage {
	    items: ConvertSourceEntry[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new ConvertSourcePage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], ConvertSourceEntry);
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
	export class ConvertSubmitRequest {
	    sourceIds: string[];
	    options: ffmpeg.ConvertOptions;
	    outputDir: string;
	    presetId: string;
	
	    static createFrom(source: any = {}) {
	        return new ConvertSubmitRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceIds = source["sourceIds"];
	        this.options = this.convertValues(source["options"], ffmpeg.ConvertOptions);
	        this.outputDir = source["outputDir"];
	        this.presetId = source["presetId"];
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
	export class Preset {
	    id: string;
	    name: string;
	    builtIn: boolean;
	    options: ffmpeg.ConvertOptions;
	
	    static createFrom(source: any = {}) {
	        return new Preset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.builtIn = source["builtIn"];
	        this.options = this.convertValues(source["options"], ffmpeg.ConvertOptions);
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
	export class PreviewURL {
	    url: string;
	    mime: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new PreviewURL(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.mime = source["mime"];
	        this.size = source["size"];
	    }
	}
	export class SourcePathCheck {
	    sourceId: string;
	    found: boolean;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SourcePathCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceId = source["sourceId"];
	        this.found = source["found"];
	        this.exists = source["exists"];
	    }
	}

}

export namespace doc {
	
	export class DocLimits {
	    maxInputsPerSubmit: number;
	    maxInputBytes: number;
	    maxPages: number;
	    maxPdfBytes: number;
	    chunkBytes: number;
	    wholeLoadBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new DocLimits(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maxInputsPerSubmit = source["maxInputsPerSubmit"];
	        this.maxInputBytes = source["maxInputBytes"];
	        this.maxPages = source["maxPages"];
	        this.maxPdfBytes = source["maxPdfBytes"];
	        this.chunkBytes = source["chunkBytes"];
	        this.wholeLoadBytes = source["wholeLoadBytes"];
	    }
	}
	export class DocFont {
	    available: boolean;
	    name: string;
	    cjk: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DocFont(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.name = source["name"];
	        this.cjk = source["cjk"];
	    }
	}
	export class DocFormat {
	    ext: string;
	    supported: boolean;
	    fidelity: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new DocFormat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ext = source["ext"];
	        this.supported = source["supported"];
	        this.fidelity = source["fidelity"];
	        this.reason = source["reason"];
	    }
	}
	export class DocCapabilities {
	    formats: DocFormat[];
	    font: DocFont;
	    limits: DocLimits;
	    experimental: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DocCapabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.formats = this.convertValues(source["formats"], DocFormat);
	        this.font = this.convertValues(source["font"], DocFont);
	        this.limits = this.convertValues(source["limits"], DocLimits);
	        this.experimental = source["experimental"];
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
	
	
	
	export class PDFChunk {
	    offset: number;
	    length: number;
	    eof: boolean;
	    size: number;
	    data: string;
	
	    static createFrom(source: any = {}) {
	        return new PDFChunk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offset = source["offset"];
	        this.length = source["length"];
	        this.eof = source["eof"];
	        this.size = source["size"];
	        this.data = source["data"];
	    }
	}
	export class PDFFile {
	    id: string;
	    path: string;
	    name: string;
	    size: number;
	    openedAt: number;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PDFFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.openedAt = source["openedAt"];
	        this.exists = source["exists"];
	    }
	}
	export class PDFSource {
	    id: string;
	    path: string;
	    name: string;
	    size: number;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new PDFSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.url = source["url"];
	    }
	}

}

export namespace edit {
	
	export class AudioClip {
	    id: string;
	    path: string;
	    trackId: string;
	    startSec: number;
	    inSec: number;
	    outSec: number;
	    speed: number;
	    volume: number;
	
	    static createFrom(source: any = {}) {
	        return new AudioClip(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.trackId = source["trackId"];
	        this.startSec = source["startSec"];
	        this.inSec = source["inSec"];
	        this.outSec = source["outSec"];
	        this.speed = source["speed"];
	        this.volume = source["volume"];
	    }
	}
	export class EditExportOptions {
	    outputName: string;
	    outputDir: string;
	
	    static createFrom(source: any = {}) {
	        return new EditExportOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outputName = source["outputName"];
	        this.outputDir = source["outputDir"];
	    }
	}
	export class EditOutput {
	    format: string;
	    width: number;
	    height: number;
	    fps: number;
	
	    static createFrom(source: any = {}) {
	        return new EditOutput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.fps = source["fps"];
	    }
	}
	export class EditWarning {
	    code: string;
	    clipId?: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new EditWarning(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.clipId = source["clipId"];
	        this.message = source["message"];
	    }
	}
	export class EditPlan {
	    durationSec: number;
	    clipCount: number;
	    inputs: string[];
	    hasAudio: boolean;
	    warnings: EditWarning[];
	
	    static createFrom(source: any = {}) {
	        return new EditPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.durationSec = source["durationSec"];
	        this.clipCount = source["clipCount"];
	        this.inputs = source["inputs"];
	        this.hasAudio = source["hasAudio"];
	        this.warnings = this.convertValues(source["warnings"], EditWarning);
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
	export class GlobalEffects {
	    brightness: number;
	    contrast: number;
	    saturation: number;
	    sharpen: number;
	
	    static createFrom(source: any = {}) {
	        return new GlobalEffects(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.brightness = source["brightness"];
	        this.contrast = source["contrast"];
	        this.saturation = source["saturation"];
	        this.sharpen = source["sharpen"];
	    }
	}
	export class VideoClip {
	    id: string;
	    path: string;
	    trackId: string;
	    startSec: number;
	    inSec: number;
	    outSec: number;
	    speed: number;
	    effectPreset: string;
	    transitionToNext: string;
	    transitionDurationSec: number;
	    blur: number;
	
	    static createFrom(source: any = {}) {
	        return new VideoClip(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.trackId = source["trackId"];
	        this.startSec = source["startSec"];
	        this.inSec = source["inSec"];
	        this.outSec = source["outSec"];
	        this.speed = source["speed"];
	        this.effectPreset = source["effectPreset"];
	        this.transitionToNext = source["transitionToNext"];
	        this.transitionDurationSec = source["transitionDurationSec"];
	        this.blur = source["blur"];
	    }
	}
	export class EditProject {
	    schemaVersion: number;
	    id: string;
	    name: string;
	    sources: string[];
	    output: EditOutput;
	    videoTrack: VideoClip[];
	    audioTrack: AudioClip[];
	    effects: GlobalEffects;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new EditProject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schemaVersion = source["schemaVersion"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.sources = source["sources"];
	        this.output = this.convertValues(source["output"], EditOutput);
	        this.videoTrack = this.convertValues(source["videoTrack"], VideoClip);
	        this.audioTrack = this.convertValues(source["audioTrack"], AudioClip);
	        this.effects = this.convertValues(source["effects"], GlobalEffects);
	        this.updatedAt = source["updatedAt"];
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
	export class EditProjectMeta {
	    id: string;
	    name: string;
	    durationSec: number;
	    clipCount: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new EditProjectMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.durationSec = source["durationSec"];
	        this.clipCount = source["clipCount"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	
	
	export class LoadedProject {
	    project: EditProject;
	    missingPaths: string[];
	
	    static createFrom(source: any = {}) {
	        return new LoadedProject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project = this.convertValues(source["project"], EditProject);
	        this.missingPaths = source["missingPaths"];
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
	export class PreviewURL {
	    url: string;
	    mime: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new PreviewURL(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.mime = source["mime"];
	        this.size = source["size"];
	    }
	}

}

export namespace ffmpeg {
	
	export class ConvertOptions {
	    container: string;
	    videoCodec: string;
	    audioCodec: string;
	    width: number;
	    height: number;
	    fps: number;
	    videoBitrate: number;
	    audioBitrate: number;
	    crf: number;
	    targetSizeMb: number;
	    trimStart: number;
	    trimEnd: number;
	
	    static createFrom(source: any = {}) {
	        return new ConvertOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.container = source["container"];
	        this.videoCodec = source["videoCodec"];
	        this.audioCodec = source["audioCodec"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.fps = source["fps"];
	        this.videoBitrate = source["videoBitrate"];
	        this.audioBitrate = source["audioBitrate"];
	        this.crf = source["crf"];
	        this.targetSizeMb = source["targetSizeMb"];
	        this.trimStart = source["trimStart"];
	        this.trimEnd = source["trimEnd"];
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

export namespace live {
	
	export class CaptureCapabilities {
	    supported: boolean;
	    platform: string;
	    backend: string;
	    sessionType: string;
	    permission: string;
	    audioCapture: boolean;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new CaptureCapabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supported = source["supported"];
	        this.platform = source["platform"];
	        this.backend = source["backend"];
	        this.sessionType = source["sessionType"];
	        this.permission = source["permission"];
	        this.audioCapture = source["audioCapture"];
	        this.reason = source["reason"];
	    }
	}
	export class CaptureSource {
	    id: string;
	    kind: string;
	    title: string;
	    width: number;
	    height: number;
	
	    static createFrom(source: any = {}) {
	        return new CaptureSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.width = source["width"];
	        this.height = source["height"];
	    }
	}
	export class PushOptions {
	    width: number;
	    height: number;
	    fps: number;
	    videoBitrateKbps: number;
	    audioBitrateKbps: number;
	
	    static createFrom(source: any = {}) {
	        return new PushOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.width = source["width"];
	        this.height = source["height"];
	        this.fps = source["fps"];
	        this.videoBitrateKbps = source["videoBitrateKbps"];
	        this.audioBitrateKbps = source["audioBitrateKbps"];
	    }
	}
	export class FilePushRequest {
	    inputPath: string;
	    url: string;
	    loop: boolean;
	    options: PushOptions;
	    preview?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FilePushRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inputPath = source["inputPath"];
	        this.url = source["url"];
	        this.loop = source["loop"];
	        this.options = this.convertValues(source["options"], PushOptions);
	        this.preview = source["preview"];
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
	export class Preview {
	    data: string;
	    ts: number;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Preview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.data = source["data"];
	        this.ts = source["ts"];
	        this.active = source["active"];
	    }
	}
	export class PullPreviewRequest {
	    url: string;
	    preview?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PullPreviewRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.preview = source["preview"];
	    }
	}
	export class PullSession {
	    id: string;
	    redacted: string;
	    preview: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PullSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.redacted = source["redacted"];
	        this.preview = source["preview"];
	    }
	}
	
	export class PushURLInfo {
	    scheme: string;
	    host: string;
	    port: number;
	    redacted: string;
	
	    static createFrom(source: any = {}) {
	        return new PushURLInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scheme = source["scheme"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.redacted = source["redacted"];
	    }
	}
	export class ScreenInfo {
	    id: string;
	    name: string;
	    primary: boolean;
	    x: number;
	    y: number;
	    width: number;
	    height: number;
	    scale: number;
	
	    static createFrom(source: any = {}) {
	        return new ScreenInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.primary = source["primary"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.scale = source["scale"];
	    }
	}
	export class ScreenPushRequest {
	    url: string;
	    screenId: string;
	    hideCursor: boolean;
	    audio: string;
	    archiveDir: string;
	    options: PushOptions;
	    preview?: boolean;
	    captureSourceId: string;
	
	    static createFrom(source: any = {}) {
	        return new ScreenPushRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.screenId = source["screenId"];
	        this.hideCursor = source["hideCursor"];
	        this.audio = source["audio"];
	        this.archiveDir = source["archiveDir"];
	        this.options = this.convertValues(source["options"], PushOptions);
	        this.preview = source["preview"];
	        this.captureSourceId = source["captureSourceId"];
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
	    hasVideo: boolean;
	    hasAudio: boolean;
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
	export class ConvertSource {
	    sourceId: string;
	    path: string;
	    name: string;
	    addedAt: number;
	    lastActivityAt: number;
	    media?: MediaInfo;
	
	    static createFrom(source: any = {}) {
	        return new ConvertSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceId = source["sourceId"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.addedAt = source["addedAt"];
	        this.lastActivityAt = source["lastActivityAt"];
	        this.media = this.convertValues(source["media"], MediaInfo);
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
	
	
	export class TaskResult {
	    sizeBytes: number;
	    durationSec?: number;
	    width?: number;
	    height?: number;
	    audioBitrateKbps?: number;
	
	    static createFrom(source: any = {}) {
	        return new TaskResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sizeBytes = source["sizeBytes"];
	        this.durationSec = source["durationSec"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.audioBitrateKbps = source["audioBitrateKbps"];
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
	    fps?: number;
	    bitrateKbps?: number;
	    droppedFrames?: number;
	    encoder?: string;
	    encoderDevice?: string;
	    hwFallback?: boolean;
	    hwFallbackReason?: string;
	    params: string;
	    version: number;
	    error?: apperr.AppError;
	    createdAt: number;
	    startedAt: number;
	    finishedAt: number;
	    sourceId?: string;
	    hiddenInTaskCenter: boolean;
	    result?: TaskResult;
	
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
	        this.fps = source["fps"];
	        this.bitrateKbps = source["bitrateKbps"];
	        this.droppedFrames = source["droppedFrames"];
	        this.encoder = source["encoder"];
	        this.encoderDevice = source["encoderDevice"];
	        this.hwFallback = source["hwFallback"];
	        this.hwFallbackReason = source["hwFallbackReason"];
	        this.params = source["params"];
	        this.version = source["version"];
	        this.error = this.convertValues(source["error"], apperr.AppError);
	        this.createdAt = source["createdAt"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.sourceId = source["sourceId"];
	        this.hiddenInTaskCenter = source["hiddenInTaskCenter"];
	        this.result = this.convertValues(source["result"], TaskResult);
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
	    includeHidden?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TaskFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.types = source["types"];
	        this.statuses = source["statuses"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	        this.includeHidden = source["includeHidden"];
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
	
	export class EncoderNames {
	    h264: string;
	    hevc: string;
	
	    static createFrom(source: any = {}) {
	        return new EncoderNames(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.h264 = source["h264"];
	        this.hevc = source["hevc"];
	    }
	}
	export class EncoderDevice {
	    id: string;
	    name: string;
	    vendor: string;
	    kind: string;
	    discrete: boolean;
	    encoders: EncoderNames;
	    available: boolean;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new EncoderDevice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.vendor = source["vendor"];
	        this.kind = source["kind"];
	        this.discrete = source["discrete"];
	        this.encoders = this.convertValues(source["encoders"], EncoderNames);
	        this.available = source["available"];
	        this.reason = source["reason"];
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
	export class EncoderDeviceList {
	    ffmpegReady: boolean;
	    devices: EncoderDevice[];
	
	    static createFrom(source: any = {}) {
	        return new EncoderDeviceList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ffmpegReady = source["ffmpegReady"];
	        this.devices = this.convertValues(source["devices"], EncoderDevice);
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
	
	export class EncoderPreferenceInfo {
	    id: string;
	    name: string;
	    available: boolean;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new EncoderPreferenceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.available = source["available"];
	        this.reason = source["reason"];
	    }
	}
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
	export class FileFilter {
	    name: string;
	    patterns: string[];
	
	    static createFrom(source: any = {}) {
	        return new FileFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.patterns = source["patterns"];
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

export namespace task {
	
	export class DeleteFailure {
	    taskId: string;
	    path?: string;
	    reason: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteFailure(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.path = source["path"];
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	}
	export class DeleteResult {
	    deletedTaskIds: string[];
	    deletedSourceIds: string[];
	    deletedFiles: number;
	    failures: DeleteFailure[];
	
	    static createFrom(source: any = {}) {
	        return new DeleteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deletedTaskIds = source["deletedTaskIds"];
	        this.deletedSourceIds = source["deletedSourceIds"];
	        this.deletedFiles = source["deletedFiles"];
	        this.failures = this.convertValues(source["failures"], DeleteFailure);
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
	export class TaskPathCheck {
	    taskId: string;
	    found: boolean;
	    inputExists: boolean;
	    outputExists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TaskPathCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.found = source["found"];
	        this.inputExists = source["inputExists"];
	        this.outputExists = source["outputExists"];
	    }
	}

}

