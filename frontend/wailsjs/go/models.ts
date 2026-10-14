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

export namespace cat {
	
	export class AgentDefaults {
	    model: string;
	    effort: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentDefaults(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.effort = source["effort"];
	    }
	}
	export class BrowserOpenRequest {
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new BrowserOpenRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	    }
	}
	export class BrowserPage {
	    url: string;
	    title: string;
	    html: string;
	
	    static createFrom(source: any = {}) {
	        return new BrowserPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.title = source["title"];
	        this.html = source["html"];
	    }
	}
	export class CancelCloneCatProjectRequest {
	    cloneId: string;
	
	    static createFrom(source: any = {}) {
	        return new CancelCloneCatProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cloneId = source["cloneId"];
	    }
	}
	export class CatBranchesRequest {
	    convId: string;
	
	    static createFrom(source: any = {}) {
	        return new CatBranchesRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	    }
	}
	export class CatBranchesResult {
	    root: string;
	    git: boolean;
	    repo: boolean;
	    branch: string;
	    detached: boolean;
	    upstream: string;
	    locals: string[];
	    remotes: string[];
	
	    static createFrom(source: any = {}) {
	        return new CatBranchesResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.git = source["git"];
	        this.repo = source["repo"];
	        this.branch = source["branch"];
	        this.detached = source["detached"];
	        this.upstream = source["upstream"];
	        this.locals = source["locals"];
	        this.remotes = source["remotes"];
	    }
	}
	export class CatCheckoutCommitResult {
	    sha: string;
	    shortSha: string;
	    detached: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CatCheckoutCommitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sha = source["sha"];
	        this.shortSha = source["shortSha"];
	        this.detached = source["detached"];
	    }
	}
	export class CatCommitRef {
	    name: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new CatCommitRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	    }
	}
	export class CatCommit {
	    sha: string;
	    shortSha: string;
	    parents: string[];
	    subject: string;
	    authorName: string;
	    authorEmail: string;
	    authorTime: number;
	    refs: CatCommitRef[];
	    isHead: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CatCommit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sha = source["sha"];
	        this.shortSha = source["shortSha"];
	        this.parents = source["parents"];
	        this.subject = source["subject"];
	        this.authorName = source["authorName"];
	        this.authorEmail = source["authorEmail"];
	        this.authorTime = source["authorTime"];
	        this.refs = this.convertValues(source["refs"], CatCommitRef);
	        this.isHead = source["isHead"];
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
	export class CatCommitFile {
	    path: string;
	    oldPath?: string;
	    status: string;
	    additions: number;
	    deletions: number;
	    binary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CatCommitFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.oldPath = source["oldPath"];
	        this.status = source["status"];
	        this.additions = source["additions"];
	        this.deletions = source["deletions"];
	        this.binary = source["binary"];
	    }
	}
	export class CatCommitDetail {
	    sha: string;
	    parents: string[];
	    authorName: string;
	    authorEmail: string;
	    authorTime: number;
	    committerName: string;
	    committerEmail: string;
	    commitTime: number;
	    message: string;
	    files: CatCommitFile[];
	    filesTruncated: boolean;
	    diffBase: string;
	
	    static createFrom(source: any = {}) {
	        return new CatCommitDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sha = source["sha"];
	        this.parents = source["parents"];
	        this.authorName = source["authorName"];
	        this.authorEmail = source["authorEmail"];
	        this.authorTime = source["authorTime"];
	        this.committerName = source["committerName"];
	        this.committerEmail = source["committerEmail"];
	        this.commitTime = source["commitTime"];
	        this.message = source["message"];
	        this.files = this.convertValues(source["files"], CatCommitFile);
	        this.filesTruncated = source["filesTruncated"];
	        this.diffBase = source["diffBase"];
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
	
	export class CatCommitFileDiffRequest {
	    convId: string;
	    sha: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new CatCommitFileDiffRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.sha = source["sha"];
	        this.path = source["path"];
	    }
	}
	export class CatCommitPage {
	    commits: CatCommit[];
	    nextCursor?: string;
	    total?: number;
	    noCommits: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CatCommitPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commits = this.convertValues(source["commits"], CatCommit);
	        this.nextCursor = source["nextCursor"];
	        this.total = source["total"];
	        this.noCommits = source["noCommits"];
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
	
	export class CatCommitRequest {
	    convId: string;
	    sha: string;
	
	    static createFrom(source: any = {}) {
	        return new CatCommitRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.sha = source["sha"];
	    }
	}
	export class CatProviderModelList {
	    state: string;
	    models: catagent.Model[];
	    fetchedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new CatProviderModelList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.models = this.convertValues(source["models"], catagent.Model);
	        this.fetchedAt = source["fetchedAt"];
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
	export class CatTurnState {
	    turnId: string;
	    status: string;
	    retryable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CatTurnState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.turnId = source["turnId"];
	        this.status = source["status"];
	        this.retryable = source["retryable"];
	    }
	}
	export class ClearProviderKeyRequest {
	    agentKind: string;
	
	    static createFrom(source: any = {}) {
	        return new ClearProviderKeyRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agentKind = source["agentKind"];
	    }
	}
	export class CloneCatProjectRequest {
	    url: string;
	    parentDir?: string;
	    name?: string;
	
	    static createFrom(source: any = {}) {
	        return new CloneCatProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.parentDir = source["parentDir"];
	        this.name = source["name"];
	    }
	}
	export class Project {
	    id: string;
	    name: string;
	    path: string;
	    createdAt: number;
	    updatedAt: number;
	    missing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.missing = source["missing"];
	    }
	}
	export class CloneCatProjectResult {
	    cloneId: string;
	    status: string;
	    project?: Project;
	
	    static createFrom(source: any = {}) {
	        return new CloneCatProjectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cloneId = source["cloneId"];
	        this.status = source["status"];
	        this.project = this.convertValues(source["project"], Project);
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
	export class ConversationDetail {
	    id: string;
	    title: string;
	    agentKind: string;
	    accessMode: string;
	    projectPath: string;
	    projectId?: string;
	    createdAt: number;
	    updatedAt: number;
	    messages: store.CatMessage[];
	    lastTurn?: CatTurnState;
	    historyStatus?: string;
	    contextUsed?: number;
	    contextWindow?: number;
	
	    static createFrom(source: any = {}) {
	        return new ConversationDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.agentKind = source["agentKind"];
	        this.accessMode = source["accessMode"];
	        this.projectPath = source["projectPath"];
	        this.projectId = source["projectId"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.messages = this.convertValues(source["messages"], store.CatMessage);
	        this.lastTurn = this.convertValues(source["lastTurn"], CatTurnState);
	        this.historyStatus = source["historyStatus"];
	        this.contextUsed = source["contextUsed"];
	        this.contextWindow = source["contextWindow"];
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
	export class CreateConversationRequest {
	    agentKind: string;
	    title: string;
	    projectPath: string;
	    accessMode: string;
	    projectId?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateConversationRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agentKind = source["agentKind"];
	        this.title = source["title"];
	        this.projectPath = source["projectPath"];
	        this.accessMode = source["accessMode"];
	        this.projectId = source["projectId"];
	    }
	}
	export class CreateProjectRequest {
	    path: string;
	    name?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	    }
	}
	export class CreateProjectResult {
	    project: Project;
	    existed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CreateProjectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project = this.convertValues(source["project"], Project);
	        this.existed = source["existed"];
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
	export class DeleteProjectRequest {
	    id: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	    }
	}
	export class FileEntry {
	    name: string;
	    relPath: string;
	    isDir: boolean;
	    size: number;
	    modTime: number;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.relPath = source["relPath"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	    }
	}
	export class GitBranchRequest {
	    convId: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new GitBranchRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.name = source["name"];
	    }
	}
	export class GitCatCheckoutRequest {
	    convId: string;
	    name: string;
	    force?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GitCatCheckoutRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.name = source["name"];
	        this.force = source["force"];
	    }
	}
	export class GitCatConflictAbortRequest {
	    convId: string;
	
	    static createFrom(source: any = {}) {
	        return new GitCatConflictAbortRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	    }
	}
	export class GitCatConflictCompleteRequest {
	    convId: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new GitCatConflictCompleteRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.message = source["message"];
	    }
	}
	export class GitCatConflictResolveRequest {
	    convId: string;
	    path: string;
	    mode: string;
	    content?: string;
	
	    static createFrom(source: any = {}) {
	        return new GitCatConflictResolveRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.path = source["path"];
	        this.mode = source["mode"];
	        this.content = source["content"];
	    }
	}
	export class GitCatMergeRequest {
	    convId: string;
	    branch: string;
	    remote?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GitCatMergeRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.branch = source["branch"];
	        this.remote = source["remote"];
	    }
	}
	export class GitCatMergeResult {
	    status: string;
	    head?: string;
	    files?: string[];
	
	    static createFrom(source: any = {}) {
	        return new GitCatMergeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.head = source["head"];
	        this.files = source["files"];
	    }
	}
	export class GitChange {
	    path: string;
	    name: string;
	    dir: string;
	    letter: string;
	    added: number;
	    deleted: number;
	    staged: boolean;
	    unstaged: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GitChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.dir = source["dir"];
	        this.letter = source["letter"];
	        this.added = source["added"];
	        this.deleted = source["deleted"];
	        this.staged = source["staged"];
	        this.unstaged = source["unstaged"];
	    }
	}
	export class GitConflictEntry {
	    path: string;
	    ours: boolean;
	    theirs: boolean;
	    letter?: string;
	
	    static createFrom(source: any = {}) {
	        return new GitConflictEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.ours = source["ours"];
	        this.theirs = source["theirs"];
	        this.letter = source["letter"];
	    }
	}
	export class GitDiffRequest {
	    convId: string;
	    relPath: string;
	
	    static createFrom(source: any = {}) {
	        return new GitDiffRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.relPath = source["relPath"];
	    }
	}
	export class GitDiffResult {
	    path: string;
	    text: string;
	    binary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GitDiffResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.text = source["text"];
	        this.binary = source["binary"];
	    }
	}
	export class GitPRResult {
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new GitPRResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	    }
	}
	export class GitPathsRequest {
	    convId: string;
	    paths: string[];
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new GitPathsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.paths = source["paths"];
	        this.message = source["message"];
	    }
	}
	export class GitRemoteRequest {
	    convId: string;
	    op: string;
	
	    static createFrom(source: any = {}) {
	        return new GitRemoteRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.op = source["op"];
	    }
	}
	export class GitStatusRequest {
	    convId: string;
	
	    static createFrom(source: any = {}) {
	        return new GitStatusRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	    }
	}
	export class GitStatusResult {
	    repo: boolean;
	    branch: string;
	    upstream: string;
	    ahead: number;
	    behind: number;
	    remote: string;
	    web: string;
	    changes: GitChange[];
	    mergeState: string;
	    conflicts: GitConflictEntry[];
	    detached: boolean;
	    headSha: string;
	    headShortSha: string;
	
	    static createFrom(source: any = {}) {
	        return new GitStatusResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo = source["repo"];
	        this.branch = source["branch"];
	        this.upstream = source["upstream"];
	        this.ahead = source["ahead"];
	        this.behind = source["behind"];
	        this.remote = source["remote"];
	        this.web = source["web"];
	        this.changes = this.convertValues(source["changes"], GitChange);
	        this.mergeState = source["mergeState"];
	        this.conflicts = this.convertValues(source["conflicts"], GitConflictEntry);
	        this.detached = source["detached"];
	        this.headSha = source["headSha"];
	        this.headShortSha = source["headShortSha"];
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
	export class InspectRequest {
	    projectId: string;
	    agentKind?: string;
	
	    static createFrom(source: any = {}) {
	        return new InspectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.agentKind = source["agentKind"];
	    }
	}
	export class ListCatCommitsRequest {
	    convId: string;
	    cursor?: string;
	    limit?: number;
	    query?: string;
	    branch?: string;
	
	    static createFrom(source: any = {}) {
	        return new ListCatCommitsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.cursor = source["cursor"];
	        this.limit = source["limit"];
	        this.query = source["query"];
	        this.branch = source["branch"];
	    }
	}
	export class ListCatProviderModelsRequest {
	    agentKind: string;
	    refresh: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ListCatProviderModelsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agentKind = source["agentKind"];
	        this.refresh = source["refresh"];
	    }
	}
	export class ListFilesRequest {
	    convId: string;
	    relPath: string;
	
	    static createFrom(source: any = {}) {
	        return new ListFilesRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.relPath = source["relPath"];
	    }
	}
	export class ListFilesResult {
	    root: string;
	    entries: FileEntry[];
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ListFilesResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.entries = this.convertValues(source["entries"], FileEntry);
	        this.truncated = source["truncated"];
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
	export class ListScheduleRunsRequest {
	    scheduleId: string;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new ListScheduleRunsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scheduleId = source["scheduleId"];
	        this.limit = source["limit"];
	    }
	}
	export class LiveTurn {
	    convId: string;
	    turnId: string;
	
	    static createFrom(source: any = {}) {
	        return new LiveTurn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.turnId = source["turnId"];
	    }
	}
	export class MediaPreview {
	    url: string;
	    mime: string;
	    kind: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new MediaPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.mime = source["mime"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	    }
	}
	export class NotifyStatus {
	    available: boolean;
	    authorized: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NotifyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.authorized = source["authorized"];
	    }
	}
	
	export class ProviderCapability {
	    agentKind: string;
	    defaultProtocol: string;
	    allowProtocolPick: boolean;
	    allowedProtocols: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProviderCapability(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agentKind = source["agentKind"];
	        this.defaultProtocol = source["defaultProtocol"];
	        this.allowProtocolPick = source["allowProtocolPick"];
	        this.allowedProtocols = source["allowedProtocols"];
	    }
	}
	export class ProviderSettings {
	    enabledCustom: boolean;
	    protocol: string;
	    apiUrl: string;
	    apiKeySet: boolean;
	    apiKey?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabledCustom = source["enabledCustom"];
	        this.protocol = source["protocol"];
	        this.apiUrl = source["apiUrl"];
	        this.apiKeySet = source["apiKeySet"];
	        this.apiKey = source["apiKey"];
	    }
	}
	export class ReadFileRequest {
	    convId: string;
	    relPath: string;
	
	    static createFrom(source: any = {}) {
	        return new ReadFileRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.relPath = source["relPath"];
	    }
	}
	export class ReadFileResult {
	    relPath: string;
	    name: string;
	    kind: string;
	    size: number;
	    content: string;
	    dataBase64: string;
	    mime: string;
	    language: string;
	    editable: boolean;
	    modTime: number;
	
	    static createFrom(source: any = {}) {
	        return new ReadFileResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.relPath = source["relPath"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.size = source["size"];
	        this.content = source["content"];
	        this.dataBase64 = source["dataBase64"];
	        this.mime = source["mime"];
	        this.language = source["language"];
	        this.editable = source["editable"];
	        this.modTime = source["modTime"];
	    }
	}
	export class RelocateProjectRequest {
	    id: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new RelocateProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	    }
	}
	export class RenameProjectRequest {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new RenameProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class RetryCatTurnRequest {
	    conversationId: string;
	    turnId: string;
	
	    static createFrom(source: any = {}) {
	        return new RetryCatTurnRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversationId = source["conversationId"];
	        this.turnId = source["turnId"];
	    }
	}
	export class RevealConversationFolderRequest {
	    convId: string;
	
	    static createFrom(source: any = {}) {
	        return new RevealConversationFolderRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	    }
	}
	export class RevealProjectRequest {
	    id: string;
	
	    static createFrom(source: any = {}) {
	        return new RevealProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	    }
	}
	export class SaveProviderKeyRequest {
	    agentKind: string;
	    apiKey: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveProviderKeyRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agentKind = source["agentKind"];
	        this.apiKey = source["apiKey"];
	    }
	}
	export class ScheduleRun {
	    id: string;
	    scheduleId: string;
	    conversationId?: string;
	    status: string;
	    error?: apperr.AppError;
	    scheduledFor: number;
	    startedAt?: number;
	    finishedAt?: number;
	    manual: boolean;
	    missedReason?: string;
	    queuePosition?: number;
	
	    static createFrom(source: any = {}) {
	        return new ScheduleRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.scheduleId = source["scheduleId"];
	        this.conversationId = source["conversationId"];
	        this.status = source["status"];
	        this.error = this.convertValues(source["error"], apperr.AppError);
	        this.scheduledFor = source["scheduledFor"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.manual = source["manual"];
	        this.missedReason = source["missedReason"];
	        this.queuePosition = source["queuePosition"];
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
	export class Schedule {
	    id: string;
	    name: string;
	    prompt: string;
	    projectId: string;
	    frequency: string;
	    time: string;
	    weekdays: number[];
	    runAt: number;
	    timezone: string;
	    model: string;
	    effort: string;
	    approvalMode: string;
	    agentKind: string;
	    enabled: boolean;
	    projectDeleted: boolean;
	    nextRunAt?: number;
	    lastRun?: ScheduleRun;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Schedule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.prompt = source["prompt"];
	        this.projectId = source["projectId"];
	        this.frequency = source["frequency"];
	        this.time = source["time"];
	        this.weekdays = source["weekdays"];
	        this.runAt = source["runAt"];
	        this.timezone = source["timezone"];
	        this.model = source["model"];
	        this.effort = source["effort"];
	        this.approvalMode = source["approvalMode"];
	        this.agentKind = source["agentKind"];
	        this.enabled = source["enabled"];
	        this.projectDeleted = source["projectDeleted"];
	        this.nextRunAt = source["nextRunAt"];
	        this.lastRun = this.convertValues(source["lastRun"], ScheduleRun);
	        this.createdAt = source["createdAt"];
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
	export class ScheduleIDRequest {
	    id: string;
	
	    static createFrom(source: any = {}) {
	        return new ScheduleIDRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	    }
	}
	export class ScheduleInput {
	    name: string;
	    prompt: string;
	    projectId: string;
	    frequency: string;
	    time: string;
	    weekdays: number[];
	    runAt: number;
	    model: string;
	    effort: string;
	    approvalMode: string;
	    agentKind: string;
	
	    static createFrom(source: any = {}) {
	        return new ScheduleInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.prompt = source["prompt"];
	        this.projectId = source["projectId"];
	        this.frequency = source["frequency"];
	        this.time = source["time"];
	        this.weekdays = source["weekdays"];
	        this.runAt = source["runAt"];
	        this.model = source["model"];
	        this.effort = source["effort"];
	        this.approvalMode = source["approvalMode"];
	        this.agentKind = source["agentKind"];
	    }
	}
	
	export class SearchFilesRequest {
	    convId: string;
	    query: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchFilesRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.query = source["query"];
	        this.kind = source["kind"];
	    }
	}
	export class SearchHit {
	    name: string;
	    relPath: string;
	    isDir: boolean;
	    line?: number;
	    text?: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchHit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.relPath = source["relPath"];
	        this.isDir = source["isDir"];
	        this.line = source["line"];
	        this.text = source["text"];
	    }
	}
	export class SearchFilesResult {
	    hits: SearchHit[];
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SearchFilesResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hits = this.convertValues(source["hits"], SearchHit);
	        this.truncated = source["truncated"];
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
	
	export class SendAttachment {
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new SendAttachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}
	export class SendMessageRequest {
	    conversationId: string;
	    content: string;
	    modelId: string;
	    thinkLevelId: string;
	    permissionMode: string;
	    projectPath: string;
	    attachments?: SendAttachment[];
	
	    static createFrom(source: any = {}) {
	        return new SendMessageRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversationId = source["conversationId"];
	        this.content = source["content"];
	        this.modelId = source["modelId"];
	        this.thinkLevelId = source["thinkLevelId"];
	        this.permissionMode = source["permissionMode"];
	        this.projectPath = source["projectPath"];
	        this.attachments = this.convertValues(source["attachments"], SendAttachment);
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
	export class SendMessageResult {
	    userMessage: store.CatMessage;
	    turnId: string;
	    assistantMessage?: store.CatMessage;
	    error?: apperr.AppError;
	
	    static createFrom(source: any = {}) {
	        return new SendMessageResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userMessage = this.convertValues(source["userMessage"], store.CatMessage);
	        this.turnId = source["turnId"];
	        this.assistantMessage = this.convertValues(source["assistantMessage"], store.CatMessage);
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
	export class SetInspectItemEnabledRequest {
	    projectId: string;
	    agentKind?: string;
	    category: string;
	    name: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SetInspectItemEnabledRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.agentKind = source["agentKind"];
	        this.category = source["category"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Settings {
	    defaultAgentKind: string;
	    defaultsByAgent: Record<string, AgentDefaults>;
	    providersByAgent: Record<string, ProviderSettings>;
	    defaultModeByAgent: Record<string, string>;
	    customInstruction: string;
	    notifyOnFinish?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultAgentKind = source["defaultAgentKind"];
	        this.defaultsByAgent = this.convertValues(source["defaultsByAgent"], AgentDefaults, true);
	        this.providersByAgent = this.convertValues(source["providersByAgent"], ProviderSettings, true);
	        this.defaultModeByAgent = source["defaultModeByAgent"];
	        this.customInstruction = source["customInstruction"];
	        this.notifyOnFinish = source["notifyOnFinish"];
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
	export class SkillsDirRequest {
	    projectId: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillsDirRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	    }
	}
	export class SkillsDirResult {
	    skillsRoot: string;
	    mcpConfigPath: string;
	    skillsRootExists: boolean;
	    opened: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SkillsDirResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.skillsRoot = source["skillsRoot"];
	        this.mcpConfigPath = source["mcpConfigPath"];
	        this.skillsRootExists = source["skillsRootExists"];
	        this.opened = source["opened"];
	    }
	}
	export class SwitchCatBranchRequest {
	    convId: string;
	    branch: string;
	    remote?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SwitchCatBranchRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.branch = source["branch"];
	        this.remote = source["remote"];
	    }
	}
	export class TermResizeRequest {
	    id: string;
	    cols: number;
	    rows: number;
	
	    static createFrom(source: any = {}) {
	        return new TermResizeRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.cols = source["cols"];
	        this.rows = source["rows"];
	    }
	}
	export class TermStartRequest {
	    convId: string;
	
	    static createFrom(source: any = {}) {
	        return new TermStartRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	    }
	}
	export class TermStartResult {
	    id: string;
	    console: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TermStartResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.console = source["console"];
	    }
	}
	export class TermStopRequest {
	    id: string;
	
	    static createFrom(source: any = {}) {
	        return new TermStopRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	    }
	}
	export class TermWriteRequest {
	    id: string;
	    data: string;
	
	    static createFrom(source: any = {}) {
	        return new TermWriteRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.data = source["data"];
	    }
	}
	export class UpdateScheduleRequest {
	    id: string;
	    name: string;
	    prompt: string;
	    projectId: string;
	    frequency: string;
	    time: string;
	    weekdays: number[];
	    runAt: number;
	    model: string;
	    effort: string;
	    approvalMode: string;
	    agentKind: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateScheduleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.prompt = source["prompt"];
	        this.projectId = source["projectId"];
	        this.frequency = source["frequency"];
	        this.time = source["time"];
	        this.weekdays = source["weekdays"];
	        this.runAt = source["runAt"];
	        this.model = source["model"];
	        this.effort = source["effort"];
	        this.approvalMode = source["approvalMode"];
	        this.agentKind = source["agentKind"];
	    }
	}
	export class WriteFileRequest {
	    convId: string;
	    relPath: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new WriteFileRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.relPath = source["relPath"];
	        this.content = source["content"];
	    }
	}
	export class WriteFileResult {
	    relPath: string;
	    size: number;
	    modTime: number;
	
	    static createFrom(source: any = {}) {
	        return new WriteFileResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.relPath = source["relPath"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	    }
	}

}

export namespace catagent {
	
	export class AgentMode {
	    id: string;
	    label: string;
	    description: string;
	    writesFiles: boolean;
	    isDefault: boolean;
	    shortLabel: string;
	    risk: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentMode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.description = source["description"];
	        this.writesFiles = source["writesFiles"];
	        this.isDefault = source["isDefault"];
	        this.shortLabel = source["shortLabel"];
	        this.risk = source["risk"];
	    }
	}
	export class AgentStatus {
	    agentKind: string;
	    ready: boolean;
	    version?: string;
	    detail?: string;
	    modes: AgentMode[];
	    checking: boolean;
	    checkedAt?: number;
	    stale?: boolean;
	    exePath?: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agentKind = source["agentKind"];
	        this.ready = source["ready"];
	        this.version = source["version"];
	        this.detail = source["detail"];
	        this.modes = this.convertValues(source["modes"], AgentMode);
	        this.checking = source["checking"];
	        this.checkedAt = source["checkedAt"];
	        this.stale = source["stale"];
	        this.exePath = source["exePath"];
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
	export class CancelCatTurnRequest {
	    convId: string;
	    turnId: string;
	
	    static createFrom(source: any = {}) {
	        return new CancelCatTurnRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.turnId = source["turnId"];
	    }
	}
	export class InspectItem {
	    name: string;
	    description?: string;
	    source?: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new InspectItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.source = source["source"];
	        this.enabled = source["enabled"];
	    }
	}
	export class InspectResult {
	    skills: InspectItem[];
	    plugins: InspectItem[];
	    hooks: InspectItem[];
	    mcpServers: InspectItem[];
	    agents: InspectItem[];
	    version?: string;
	    reason?: string;
	    skillsRoot?: string;
	    mcpConfigPath?: string;
	    skillsRootExists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new InspectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.skills = this.convertValues(source["skills"], InspectItem);
	        this.plugins = this.convertValues(source["plugins"], InspectItem);
	        this.hooks = this.convertValues(source["hooks"], InspectItem);
	        this.mcpServers = this.convertValues(source["mcpServers"], InspectItem);
	        this.agents = this.convertValues(source["agents"], InspectItem);
	        this.version = source["version"];
	        this.reason = source["reason"];
	        this.skillsRoot = source["skillsRoot"];
	        this.mcpConfigPath = source["mcpConfigPath"];
	        this.skillsRootExists = source["skillsRootExists"];
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
	export class Model {
	    id: string;
	    displayName: string;
	    efforts: string[];
	    defaultEffort: string;
	
	    static createFrom(source: any = {}) {
	        return new Model(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.displayName = source["displayName"];
	        this.efforts = source["efforts"];
	        this.defaultEffort = source["defaultEffort"];
	    }
	}
	export class Status {
	    state: string;
	    version: string;
	    canDownload: boolean;
	    error?: apperr.AppError;
	    agents: AgentStatus[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.version = source["version"];
	        this.canDownload = source["canDownload"];
	        this.error = this.convertValues(source["error"], apperr.AppError);
	        this.agents = this.convertValues(source["agents"], AgentStatus);
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
	export class ThinkLevel {
	    id: string;
	    displayName: string;
	
	    static createFrom(source: any = {}) {
	        return new ThinkLevel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.displayName = source["displayName"];
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
	export class SkippedSource {
	    sourceId: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new SkippedSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceId = source["sourceId"];
	        this.reason = source["reason"];
	    }
	}
	export class ConvertSubmitResult {
	    tasks: store.Task[];
	    skipped: SkippedSource[];
	
	    static createFrom(source: any = {}) {
	        return new ConvertSubmitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tasks = this.convertValues(source["tasks"], store.Task);
	        this.skipped = this.convertValues(source["skipped"], SkippedSource);
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
	export class FormatPreset {
	    id: string;
	    name: string;
	    builtIn: boolean;
	    paramsSummary: string;
	    options: ffmpeg.ConvertOptions;
	
	    static createFrom(source: any = {}) {
	        return new FormatPreset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.builtIn = source["builtIn"];
	        this.paramsSummary = source["paramsSummary"];
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
	export class FormatEntry {
	    category: string;
	    extension: string;
	    displayName: string;
	    aliases: string[];
	    encodable: boolean;
	    reason?: string;
	    reasonCode?: string;
	    defaultPresetId: string;
	    presets: FormatPreset[];
	
	    static createFrom(source: any = {}) {
	        return new FormatEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.extension = source["extension"];
	        this.displayName = source["displayName"];
	        this.aliases = source["aliases"];
	        this.encodable = source["encodable"];
	        this.reason = source["reason"];
	        this.reasonCode = source["reasonCode"];
	        this.defaultPresetId = source["defaultPresetId"];
	        this.presets = this.convertValues(source["presets"], FormatPreset);
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
	export class ReconvertRequest {
	    taskId: string;
	    presetId?: string;
	    options?: ffmpeg.ConvertOptions;
	
	    static createFrom(source: any = {}) {
	        return new ReconvertRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.presetId = source["presetId"];
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
	
	export class SourcePathCheck {
	    sourceId: string;
	    found: boolean;
	    exists: boolean;
	    originalExists: boolean;
	    storedExists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SourcePathCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceId = source["sourceId"];
	        this.found = source["found"];
	        this.exists = source["exists"];
	        this.originalExists = source["originalExists"];
	        this.storedExists = source["storedExists"];
	    }
	}

}

export namespace doc {
	
	export class DocSource {
	    sourceId: string;
	    path: string;
	    name: string;
	    addedAt: number;
	    lastActivityAt: number;
	    media?: store.MediaInfo;
	    originalPath: string;
	    storedPath: string;
	    copyState: string;
	    copiedBytes: number;
	    totalBytes: number;
	    copyError?: apperr.AppError;
	    ext: string;
	    family: string;
	    sheetCount: number;
	
	    static createFrom(source: any = {}) {
	        return new DocSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceId = source["sourceId"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.addedAt = source["addedAt"];
	        this.lastActivityAt = source["lastActivityAt"];
	        this.media = this.convertValues(source["media"], store.MediaInfo);
	        this.originalPath = source["originalPath"];
	        this.storedPath = source["storedPath"];
	        this.copyState = source["copyState"];
	        this.copiedBytes = source["copiedBytes"];
	        this.totalBytes = source["totalBytes"];
	        this.copyError = this.convertValues(source["copyError"], apperr.AppError);
	        this.ext = source["ext"];
	        this.family = source["family"];
	        this.sheetCount = source["sheetCount"];
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
	export class AddDocSourceResult {
	    path: string;
	    source?: DocSource;
	    error?: apperr.AppError;
	
	    static createFrom(source: any = {}) {
	        return new AddDocSourceResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.source = this.convertValues(source["source"], DocSource);
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
	export class DocBinaryChunk {
	    saveId: string;
	    seq: number;
	    data: string;
	
	    static createFrom(source: any = {}) {
	        return new DocBinaryChunk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saveId = source["saveId"];
	        this.seq = source["seq"];
	        this.data = source["data"];
	    }
	}
	export class DocBinaryChunkResult {
	    receivedBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new DocBinaryChunkResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.receivedBytes = source["receivedBytes"];
	    }
	}
	export class DocBinarySaveAbort {
	    saveId: string;
	
	    static createFrom(source: any = {}) {
	        return new DocBinarySaveAbort(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saveId = source["saveId"];
	    }
	}
	export class DocBinarySaveBegin {
	    sourceId?: string;
	    taskId?: string;
	    mode: string;
	    targetPath?: string;
	    revision?: string;
	    totalBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new DocBinarySaveBegin(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceId = source["sourceId"];
	        this.taskId = source["taskId"];
	        this.mode = source["mode"];
	        this.targetPath = source["targetPath"];
	        this.revision = source["revision"];
	        this.totalBytes = source["totalBytes"];
	    }
	}
	export class DocBinarySaveCommit {
	    saveId: string;
	    sha256: string;
	
	    static createFrom(source: any = {}) {
	        return new DocBinarySaveCommit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saveId = source["saveId"];
	        this.sha256 = source["sha256"];
	    }
	}
	export class DocBinarySaveResult {
	    path: string;
	    revision: string;
	    sizeBytes: number;
	    savedAt: number;
	    backupPath?: string;
	
	    static createFrom(source: any = {}) {
	        return new DocBinarySaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.revision = source["revision"];
	        this.sizeBytes = source["sizeBytes"];
	        this.savedAt = source["savedAt"];
	        this.backupPath = source["backupPath"];
	    }
	}
	export class DocBinarySaveSession {
	    saveId: string;
	    maxChunkBytes: number;
	    expiresAt: number;
	
	    static createFrom(source: any = {}) {
	        return new DocBinarySaveSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saveId = source["saveId"];
	        this.maxChunkBytes = source["maxChunkBytes"];
	        this.expiresAt = source["expiresAt"];
	    }
	}
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
	export class DocEngineInfo {
	    id: string;
	    name: string;
	    version: string;
	    source?: string;
	    installed: boolean;
	    families: string[];
	    available: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DocEngineInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.source = source["source"];
	        this.installed = source["installed"];
	        this.families = source["families"];
	        this.available = source["available"];
	    }
	}
	export class DocComponentStatus {
	    state: string;
	    componentState: string;
	    engines: DocEngineInfo[];
	    version: string;
	    source: string;
	    canDownload: boolean;
	    downloadBytes: number;
	    installBytes: number;
	    phase?: string;
	    receivedBytes?: number;
	    error?: apperr.AppError;
	
	    static createFrom(source: any = {}) {
	        return new DocComponentStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.componentState = source["componentState"];
	        this.engines = this.convertValues(source["engines"], DocEngineInfo);
	        this.version = source["version"];
	        this.source = source["source"];
	        this.canDownload = source["canDownload"];
	        this.downloadBytes = source["downloadBytes"];
	        this.installBytes = source["installBytes"];
	        this.phase = source["phase"];
	        this.receivedBytes = source["receivedBytes"];
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
	
	
	
	export class DocTarget {
	    ext: string;
	    displayName: string;
	    needsComponent: boolean;
	    simple: boolean;
	    available: boolean;
	    hintKey?: string;
	    hint?: string;
	    disabledReason?: string;
	    engines?: string[];
	
	    static createFrom(source: any = {}) {
	        return new DocTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ext = source["ext"];
	        this.displayName = source["displayName"];
	        this.needsComponent = source["needsComponent"];
	        this.simple = source["simple"];
	        this.available = source["available"];
	        this.hintKey = source["hintKey"];
	        this.hint = source["hint"];
	        this.disabledReason = source["disabledReason"];
	        this.engines = source["engines"];
	    }
	}
	export class DocSourceFormats {
	    ext: string;
	    aliases?: string[];
	    family: string;
	    targets: DocTarget[];
	
	    static createFrom(source: any = {}) {
	        return new DocSourceFormats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ext = source["ext"];
	        this.aliases = source["aliases"];
	        this.family = source["family"];
	        this.targets = this.convertValues(source["targets"], DocTarget);
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
	export class DocFormatMatrix {
	    componentReady: boolean;
	    inputs: string[];
	    sources: DocSourceFormats[];
	
	    static createFrom(source: any = {}) {
	        return new DocFormatMatrix(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.componentReady = source["componentReady"];
	        this.inputs = source["inputs"];
	        this.sources = this.convertValues(source["sources"], DocSourceFormats);
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
	
	export class DocPreview {
	    previewId: string;
	    kind: string;
	    state: string;
	    name: string;
	    ext: string;
	    url?: string;
	    rawUrl?: string;
	    text?: string;
	    rows?: string[][];
	    totalRows?: number;
	    truncated?: boolean;
	    sizeBytes: number;
	    reason?: string;
	    error?: apperr.AppError;
	    editable: boolean;
	    editBlock?: string;
	    revision?: string;
	    encoding?: string;
	    lineEnding?: string;
	
	    static createFrom(source: any = {}) {
	        return new DocPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.previewId = source["previewId"];
	        this.kind = source["kind"];
	        this.state = source["state"];
	        this.name = source["name"];
	        this.ext = source["ext"];
	        this.url = source["url"];
	        this.rawUrl = source["rawUrl"];
	        this.text = source["text"];
	        this.rows = source["rows"];
	        this.totalRows = source["totalRows"];
	        this.truncated = source["truncated"];
	        this.sizeBytes = source["sizeBytes"];
	        this.reason = source["reason"];
	        this.error = this.convertValues(source["error"], apperr.AppError);
	        this.editable = source["editable"];
	        this.editBlock = source["editBlock"];
	        this.revision = source["revision"];
	        this.encoding = source["encoding"];
	        this.lineEnding = source["lineEnding"];
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
	export class DocPreviewRequest {
	    sourceId?: string;
	    taskId?: string;
	
	    static createFrom(source: any = {}) {
	        return new DocPreviewRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceId = source["sourceId"];
	        this.taskId = source["taskId"];
	    }
	}
	export class DocSaveAsRequest {
	    sourceId?: string;
	    taskId?: string;
	    targetPath: string;
	    text?: string;
	    rows?: string[][];
	    encoding?: string;
	
	    static createFrom(source: any = {}) {
	        return new DocSaveAsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceId = source["sourceId"];
	        this.taskId = source["taskId"];
	        this.targetPath = source["targetPath"];
	        this.text = source["text"];
	        this.rows = source["rows"];
	        this.encoding = source["encoding"];
	    }
	}
	export class DocSaveRequest {
	    sourceId?: string;
	    taskId?: string;
	    revision: string;
	    text?: string;
	    rows?: string[][];
	
	    static createFrom(source: any = {}) {
	        return new DocSaveRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceId = source["sourceId"];
	        this.taskId = source["taskId"];
	        this.revision = source["revision"];
	        this.text = source["text"];
	        this.rows = source["rows"];
	    }
	}
	export class DocSaveResult {
	    path: string;
	    revision: string;
	    sizeBytes: number;
	    savedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new DocSaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.revision = source["revision"];
	        this.sizeBytes = source["sizeBytes"];
	        this.savedAt = source["savedAt"];
	    }
	}
	
	export class DocSourceEntry {
	    source: DocSource;
	    records: store.Task[];
	    recordCount: number;
	
	    static createFrom(source: any = {}) {
	        return new DocSourceEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = this.convertValues(source["source"], DocSource);
	        this.records = this.convertValues(source["records"], store.Task);
	        this.recordCount = source["recordCount"];
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
	
	export class DocSourcePage {
	    items: DocSourceEntry[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new DocSourcePage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], DocSourceEntry);
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
	export class DocSubmitRequest {
	    sourceIds: string[];
	    target: string;
	    outputDir: string;
	
	    static createFrom(source: any = {}) {
	        return new DocSubmitRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceIds = source["sourceIds"];
	        this.target = source["target"];
	        this.outputDir = source["outputDir"];
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

export namespace lang {
	
	export class ExportSubtitleRequest {
	    taskId: string;
	    cues: langasr.SubtitleCue[];
	    format: string;
	    targetPath?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportSubtitleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.cues = this.convertValues(source["cues"], langasr.SubtitleCue);
	        this.format = source["format"];
	        this.targetPath = source["targetPath"];
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
	export class ExportSubtitleResult {
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportSubtitleResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	    }
	}
	export class SpeechToSubtitleRequest {
	    paths: string[];
	    language?: string;
	    format: string;
	    outputDir?: string;
	
	    static createFrom(source: any = {}) {
	        return new SpeechToSubtitleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.paths = source["paths"];
	        this.language = source["language"];
	        this.format = source["format"];
	        this.outputDir = source["outputDir"];
	    }
	}

}

export namespace langasr {
	
	export class Status {
	    state: string;
	    version: string;
	    source: string;
	    tier: string;
	    canDownload: boolean;
	    downloadBytes: number;
	    installBytes?: number;
	    phase?: string;
	    receivedBytes?: number;
	    error?: apperr.AppError;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.version = source["version"];
	        this.source = source["source"];
	        this.tier = source["tier"];
	        this.canDownload = source["canDownload"];
	        this.downloadBytes = source["downloadBytes"];
	        this.installBytes = source["installBytes"];
	        this.phase = source["phase"];
	        this.receivedBytes = source["receivedBytes"];
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
	export class SubtitleCue {
	    id: string;
	    text: string;
	    startMs: number;
	    endMs: number;
	
	    static createFrom(source: any = {}) {
	        return new SubtitleCue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.text = source["text"];
	        this.startMs = source["startMs"];
	        this.endMs = source["endMs"];
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
	export class PreviewStream {
	    url: string;
	    mime: string;
	    hasVideo: boolean;
	    hasAudio: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PreviewStream(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.mime = source["mime"];
	        this.hasVideo = source["hasVideo"];
	        this.hasAudio = source["hasAudio"];
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
	    previewUrl: string;
	    hasVideo?: boolean;
	    hasAudio?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PullSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.redacted = source["redacted"];
	        this.preview = source["preview"];
	        this.previewUrl = source["previewUrl"];
	        this.hasVideo = source["hasVideo"];
	        this.hasAudio = source["hasAudio"];
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

export namespace office {
	
	export class ExportOfficeCanvasPptxRequest {
	    convId: string;
	    artifactId: string;
	    destPath?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportOfficeCanvasPptxRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.convId = source["convId"];
	        this.artifactId = source["artifactId"];
	        this.destPath = source["destPath"];
	    }
	}
	export class ExportOfficeCanvasPptxResult {
	    path: string;
	    bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new ExportOfficeCanvasPptxResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.bytes = source["bytes"];
	    }
	}

}

export namespace officecanvas {
	
	export class Status {
	    state: string;
	    version: string;
	    source: string;
	    canDownload: boolean;
	    downloadBytes: number;
	    installBytes?: number;
	    phase?: string;
	    receivedBytes?: number;
	    error?: apperr.AppError;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.version = source["version"];
	        this.source = source["source"];
	        this.canDownload = source["canDownload"];
	        this.downloadBytes = source["downloadBytes"];
	        this.installBytes = source["installBytes"];
	        this.phase = source["phase"];
	        this.receivedBytes = source["receivedBytes"];
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

}

export namespace store {
	
	export class CatConversation {
	    id: string;
	    title: string;
	    agentKind: string;
	    accessMode: string;
	    projectPath: string;
	    projectId?: string;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new CatConversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.agentKind = source["agentKind"];
	        this.accessMode = source["accessMode"];
	        this.projectPath = source["projectPath"];
	        this.projectId = source["projectId"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class CatMedia {
	    kind: string;
	    name: string;
	    path?: string;
	    mime?: string;
	    data?: string;
	
	    static createFrom(source: any = {}) {
	        return new CatMedia(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.mime = source["mime"];
	        this.data = source["data"];
	    }
	}
	export class CatMessage {
	    id: string;
	    role: string;
	    content: string;
	    createdAt: number;
	    media?: CatMedia[];
	    finishedAt?: number;
	    tokenTotal?: number;
	    tokensPerSec?: number;
	    outputTokens?: number;
	    firstTokenMs?: number;
	    totalMs?: number;
	
	    static createFrom(source: any = {}) {
	        return new CatMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.createdAt = source["createdAt"];
	        this.media = this.convertValues(source["media"], CatMedia);
	        this.finishedAt = source["finishedAt"];
	        this.tokenTotal = source["tokenTotal"];
	        this.tokensPerSec = source["tokensPerSec"];
	        this.outputTokens = source["outputTokens"];
	        this.firstTokenMs = source["firstTokenMs"];
	        this.totalMs = source["totalMs"];
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
	    videoCodecName?: string;
	    audioCodecName?: string;
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
	        this.videoCodecName = source["videoCodecName"];
	        this.audioCodecName = source["audioCodecName"];
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
	    originalPath: string;
	    storedPath: string;
	    copyState: string;
	    copiedBytes: number;
	    totalBytes: number;
	    copyError?: apperr.AppError;
	
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
	        this.originalPath = source["originalPath"];
	        this.storedPath = source["storedPath"];
	        this.copyState = source["copyState"];
	        this.copiedBytes = source["copiedBytes"];
	        this.totalBytes = source["totalBytes"];
	        this.copyError = this.convertValues(source["copyError"], apperr.AppError);
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
	
	export class ReconvertError {
	    code: string;
	    message: string;
	    detail?: string;
	    at: number;
	
	    static createFrom(source: any = {}) {
	        return new ReconvertError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.message = source["message"];
	        this.detail = source["detail"];
	        this.at = source["at"];
	    }
	}
	
	export class SubtitleCue {
	    id: string;
	    text: string;
	    startMs: number;
	    endMs: number;
	
	    static createFrom(source: any = {}) {
	        return new SubtitleCue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.text = source["text"];
	        this.startMs = source["startMs"];
	        this.endMs = source["endMs"];
	    }
	}
	export class TaskResult {
	    sizeBytes: number;
	    durationSec?: number;
	    width?: number;
	    height?: number;
	    audioBitrateKbps?: number;
	    warnings?: string[];
	    engine?: string;
	    cues?: SubtitleCue[];
	
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
	        this.warnings = source["warnings"];
	        this.engine = source["engine"];
	        this.cues = this.convertValues(source["cues"], SubtitleCue);
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
	    reconverting: boolean;
	    lastReconvertError?: ReconvertError;
	    queuePosition?: number;
	
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
	        this.reconverting = source["reconverting"];
	        this.lastReconvertError = this.convertValues(source["lastReconvertError"], ReconvertError);
	        this.queuePosition = source["queuePosition"];
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
	    version: string;
	    source: string;
	    taskId?: string;
	    customPathInvalid: boolean;
	    ffprobeMissing: boolean;
	    error?: apperr.AppError;
	
	    static createFrom(source: any = {}) {
	        return new FFmpegStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.version = source["version"];
	        this.source = source["source"];
	        this.taskId = source["taskId"];
	        this.customPathInvalid = source["customPathInvalid"];
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
	    uploadsDir: string;
	    maxConcurrent: number;
	    docEngine: string;
	    asrTier: string;
	    catDefaultAgentKind: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ffmpegPath = source["ffmpegPath"];
	        this.ffmpegPromptDismissed = source["ffmpegPromptDismissed"];
	        this.defaultOutputDir = source["defaultOutputDir"];
	        this.uploadsDir = source["uploadsDir"];
	        this.maxConcurrent = source["maxConcurrent"];
	        this.docEngine = source["docEngine"];
	        this.asrTier = source["asrTier"];
	        this.catDefaultAgentKind = source["catDefaultAgentKind"];
	    }
	}
	export class StorageDirs {
	    outputDir: string;
	    uploadsDir: string;
	    outputCustom: boolean;
	    uploadsCustom: boolean;
	    defaultOutputDir: string;
	    defaultUploadsDir: string;
	    baseKind: string;
	    fellBack: boolean;
	    outputAvailable: boolean;
	    uploadsAvailable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StorageDirs(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outputDir = source["outputDir"];
	        this.uploadsDir = source["uploadsDir"];
	        this.outputCustom = source["outputCustom"];
	        this.uploadsCustom = source["uploadsCustom"];
	        this.defaultOutputDir = source["defaultOutputDir"];
	        this.defaultUploadsDir = source["defaultUploadsDir"];
	        this.baseKind = source["baseKind"];
	        this.fellBack = source["fellBack"];
	        this.outputAvailable = source["outputAvailable"];
	        this.uploadsAvailable = source["uploadsAvailable"];
	    }
	}
	export class StorageDirsUpdate {
	    outputDir: string;
	    uploadsDir: string;
	
	    static createFrom(source: any = {}) {
	        return new StorageDirsUpdate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outputDir = source["outputDir"];
	        this.uploadsDir = source["uploadsDir"];
	    }
	}

}

export namespace task {
	
	export class DeleteFailure {
	    taskId: string;
	    sourceId?: string;
	    path?: string;
	    reason: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteFailure(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.sourceId = source["sourceId"];
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
	    reconvertMode: string;
	    reconvertBlock: string;
	
	    static createFrom(source: any = {}) {
	        return new TaskPathCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.found = source["found"];
	        this.inputExists = source["inputExists"];
	        this.outputExists = source["outputExists"];
	        this.reconvertMode = source["reconvertMode"];
	        this.reconvertBlock = source["reconvertBlock"];
	    }
	}

}

