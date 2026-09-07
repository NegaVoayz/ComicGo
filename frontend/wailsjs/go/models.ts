export namespace model {
	
	export class ComicCard {
	    id: number;
	    filename: string;
	    title: string;
	    pageCount: number;
	    fav: boolean;
	    lastPage: number;
	    hasNonUtf8: boolean;
	    coverUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new ComicCard(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.filename = source["filename"];
	        this.title = source["title"];
	        this.pageCount = source["pageCount"];
	        this.fav = source["fav"];
	        this.lastPage = source["lastPage"];
	        this.hasNonUtf8 = source["hasNonUtf8"];
	        this.coverUrl = source["coverUrl"];
	    }
	}
	export class Meta {
	    id: number;
	    path: string;
	    filename: string;
	    dir: string;
	    pageCount: number;
	    fileSize: number;
	    addedAt: number;
	    updatedAt: number;
	    lastReadPage: number;
	    readAt: number;
	    fav: boolean;
	    hasNonUtf8: boolean;
	    kv: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new Meta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.filename = source["filename"];
	        this.dir = source["dir"];
	        this.pageCount = source["pageCount"];
	        this.fileSize = source["fileSize"];
	        this.addedAt = source["addedAt"];
	        this.updatedAt = source["updatedAt"];
	        this.lastReadPage = source["lastReadPage"];
	        this.readAt = source["readAt"];
	        this.fav = source["fav"];
	        this.hasNonUtf8 = source["hasNonUtf8"];
	        this.kv = source["kv"];
	    }
	}
	export class ComicDetail {
	    meta: Meta;
	    commentRaw: string;
	    allMetaKeys: string[];
	
	    static createFrom(source: any = {}) {
	        return new ComicDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.meta = this.convertValues(source["meta"], Meta);
	        this.commentRaw = source["commentRaw"];
	        this.allMetaKeys = source["allMetaKeys"];
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
	export class CoverRef {
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new CoverRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	    }
	}
	export class LayoutSection {
	    id: string;
	    title: string;
	    kind: string;
	    cardId: string;
	    filterRuleId?: string;
	    sortRuleId?: string;
	    pageSize?: number;
	
	    static createFrom(source: any = {}) {
	        return new LayoutSection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.kind = source["kind"];
	        this.cardId = source["cardId"];
	        this.filterRuleId = source["filterRuleId"];
	        this.sortRuleId = source["sortRuleId"];
	        this.pageSize = source["pageSize"];
	    }
	}
	export class LayoutDoc {
	    version: number;
	    sections: LayoutSection[];
	
	    static createFrom(source: any = {}) {
	        return new LayoutDoc(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.sections = this.convertValues(source["sections"], LayoutSection);
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
	
	
	export class OpenResult {
	    id: number;
	    pageCount: number;
	
	    static createFrom(source: any = {}) {
	        return new OpenResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.pageCount = source["pageCount"];
	    }
	}
	export class PageQuery {
	    offset: number;
	    limit: number;
	    filterRuleId: string;
	    sortRuleId: string;
	
	    static createFrom(source: any = {}) {
	        return new PageQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offset = source["offset"];
	        this.limit = source["limit"];
	        this.filterRuleId = source["filterRuleId"];
	        this.sortRuleId = source["sortRuleId"];
	    }
	}
	export class PageRef {
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new PageRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	    }
	}
	export class Progress {
	    comicId: number;
	    lastPage: number;
	    pageCount: number;
	
	    static createFrom(source: any = {}) {
	        return new Progress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.comicId = source["comicId"];
	        this.lastPage = source["lastPage"];
	        this.pageCount = source["pageCount"];
	    }
	}
	export class RuleInfo {
	    id: string;
	    label: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new RuleInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	    }
	}
	export class Settings {
	    libraryRoots: string[];
	    mediaTransport: string;
	    schemaVersion: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.libraryRoots = source["libraryRoots"];
	        this.mediaTransport = source["mediaTransport"];
	        this.schemaVersion = source["schemaVersion"];
	    }
	}

}

