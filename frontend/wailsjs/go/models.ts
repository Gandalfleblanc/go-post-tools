export namespace api {
	
	export class AdminLiensResponse {
	    pagination: struct { CurrentPage int "json:\"current_page\""; LastPage int "json:\"last_page,omitempty\""; Total int "json:\"total,omitempty\""; Data []api.;
	
	    static createFrom(source: any = {}) {
	        return new AdminLiensResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pagination = this.convertValues(source["pagination"], Object);
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
	export class AdminTorrentsResponse {
	    pagination: struct { CurrentPage int "json:\"current_page\""; LastPage int "json:\"last_page,omitempty\""; Total int "json:\"total,omitempty\""; Data []api.;
	
	    static createFrom(source: any = {}) {
	        return new AdminTorrentsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pagination = this.convertValues(source["pagination"], Object);
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
	export class HostDetails {
	    id_host: number;
	    name: string;
	    url?: string;
	    icon?: string;
	
	    static createFrom(source: any = {}) {
	        return new HostDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id_host = source["id_host"];
	        this.name = source["name"];
	        this.url = source["url"];
	        this.icon = source["icon"];
	    }
	}
	export class Lang {
	    id: number;
	    name: string;
	    code?: string;
	
	    static createFrom(source: any = {}) {
	        return new Lang(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.code = source["code"];
	    }
	}
	export class LangPivot {
	    id: number;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new LangPivot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class QualDetails {
	    id_qual: number;
	    qual: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new QualDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id_qual = source["id_qual"];
	        this.qual = source["qual"];
	        this.label = source["label"];
	    }
	}
	export class Lien {
	    id: number;
	    title_id?: number;
	    lien?: string;
	    id_host?: number;
	    qualite?: number;
	    saison?: number;
	    episode?: number;
	    full_saison?: number;
	    taille?: number;
	    id_user?: string;
	    active?: number;
	    view?: number;
	    created_at?: string;
	    updated_at?: string;
	    qual?: QualDetails;
	    host?: HostDetails;
	    langues_compact?: LangPivot[];
	    subs_compact?: LangPivot[];
	
	    static createFrom(source: any = {}) {
	        return new Lien(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title_id = source["title_id"];
	        this.lien = source["lien"];
	        this.id_host = source["id_host"];
	        this.qualite = source["qualite"];
	        this.saison = source["saison"];
	        this.episode = source["episode"];
	        this.full_saison = source["full_saison"];
	        this.taille = source["taille"];
	        this.id_user = source["id_user"];
	        this.active = source["active"];
	        this.view = source["view"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.qual = this.convertValues(source["qual"], QualDetails);
	        this.host = this.convertValues(source["host"], HostDetails);
	        this.langues_compact = this.convertValues(source["langues_compact"], LangPivot);
	        this.subs_compact = this.convertValues(source["subs_compact"], LangPivot);
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
	export class LienDetail {
	    lien: Lien;
	    directDL: string;
	    raw_url: string;
	    debrided: boolean;
	    debrid_error: string;
	    debrid_error_detail: string;
	    link_source: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new LienDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lien = this.convertValues(source["lien"], Lien);
	        this.directDL = source["directDL"];
	        this.raw_url = source["raw_url"];
	        this.debrided = source["debrided"];
	        this.debrid_error = source["debrid_error"];
	        this.debrid_error_detail = source["debrid_error_detail"];
	        this.link_source = source["link_source"];
	        this.status = source["status"];
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
	export class LiensResult {
	    liens: Lien[];
	    count: number;
	    charged: number;
	    already_paid: number;
	
	    static createFrom(source: any = {}) {
	        return new LiensResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.liens = this.convertValues(source["liens"], Lien);
	        this.count = source["count"];
	        this.charged = source["charged"];
	        this.already_paid = source["already_paid"];
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
	export class Nzb {
	    id: number;
	    title_id?: number;
	    name?: string;
	    download_url?: string;
	    qualite?: number;
	    saison?: number;
	    episode?: number;
	    size?: number;
	    id_user?: string;
	    author?: string;
	    active?: number;
	    created_at?: string;
	    updated_at?: string;
	    qual?: QualDetails;
	    langues_compact?: LangPivot[];
	    subs_compact?: LangPivot[];
	
	    static createFrom(source: any = {}) {
	        return new Nzb(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title_id = source["title_id"];
	        this.name = source["name"];
	        this.download_url = source["download_url"];
	        this.qualite = source["qualite"];
	        this.saison = source["saison"];
	        this.episode = source["episode"];
	        this.size = source["size"];
	        this.id_user = source["id_user"];
	        this.author = source["author"];
	        this.active = source["active"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.qual = this.convertValues(source["qual"], QualDetails);
	        this.langues_compact = this.convertValues(source["langues_compact"], LangPivot);
	        this.subs_compact = this.convertValues(source["subs_compact"], LangPivot);
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
	export class NzbsResult {
	    nzbs: Nzb[];
	    count: number;
	    charged: number;
	    already_paid: number;
	
	    static createFrom(source: any = {}) {
	        return new NzbsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nzbs = this.convertValues(source["nzbs"], Nzb);
	        this.count = source["count"];
	        this.charged = source["charged"];
	        this.already_paid = source["already_paid"];
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
	export class PartialTitle {
	    id: number;
	    name: string;
	    type: string;
	    poster?: string;
	    release_date?: string;
	    score?: number;
	    runtime?: number;
	    last_content_added_at?: string;
	
	    static createFrom(source: any = {}) {
	        return new PartialTitle(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.poster = source["poster"];
	        this.release_date = source["release_date"];
	        this.score = source["score"];
	        this.runtime = source["runtime"];
	        this.last_content_added_at = source["last_content_added_at"];
	    }
	}
	
	export class Quality {
	    id: number;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new Quality(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class ReseedRequestUser {
	    id: number;
	    username: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new ReseedRequestUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.username = source["username"];
	        this.name = source["name"];
	    }
	}
	export class ReseedRequestTitle {
	    id: number;
	    name: string;
	    poster: string;
	    year?: number;
	
	    static createFrom(source: any = {}) {
	        return new ReseedRequestTitle(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.poster = source["poster"];
	        this.year = source["year"];
	    }
	}
	export class ReseedRequestTorrent {
	    id: number;
	    title_id: number;
	    torrent_name: string;
	    name: string;
	    info_hash: string;
	    seeders: number;
	    author: string;
	    size?: number;
	    title: ReseedRequestTitle;
	
	    static createFrom(source: any = {}) {
	        return new ReseedRequestTorrent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title_id = source["title_id"];
	        this.torrent_name = source["torrent_name"];
	        this.name = source["name"];
	        this.info_hash = source["info_hash"];
	        this.seeders = source["seeders"];
	        this.author = source["author"];
	        this.size = source["size"];
	        this.title = this.convertValues(source["title"], ReseedRequestTitle);
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
	export class ReseedRequest {
	    id: number;
	    torrent_id: number;
	    requester_id: number;
	    uploader_id: number;
	    status: string;
	    created_at: string;
	    updated_at: string;
	    torrent: ReseedRequestTorrent;
	    requester: ReseedRequestUser;
	    uploader: ReseedRequestUser;
	
	    static createFrom(source: any = {}) {
	        return new ReseedRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.torrent_id = source["torrent_id"];
	        this.requester_id = source["requester_id"];
	        this.uploader_id = source["uploader_id"];
	        this.status = source["status"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.torrent = this.convertValues(source["torrent"], ReseedRequestTorrent);
	        this.requester = this.convertValues(source["requester"], ReseedRequestUser);
	        this.uploader = this.convertValues(source["uploader"], ReseedRequestUser);
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
	
	
	
	export class ReseedRequestsResponse {
	    pagination: struct { CurrentPage int "json:\"current_page\""; LastPage int "json:\"last_page,omitempty\""; Total int "json:\"total,omitempty\""; Data []api.;
	
	    static createFrom(source: any = {}) {
	        return new ReseedRequestsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pagination = this.convertValues(source["pagination"], Object);
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
	export class TitlesResponse {
	    current_page: number;
	    per_page: number;
	    total: number;
	    last_page: number;
	    data: PartialTitle[];
	
	    static createFrom(source: any = {}) {
	        return new TitlesResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current_page = source["current_page"];
	        this.per_page = source["per_page"];
	        this.total = source["total"];
	        this.last_page = source["last_page"];
	        this.data = this.convertValues(source["data"], PartialTitle);
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
	export class TorrentItem {
	    id: number;
	    title_id?: number;
	    torrent_name?: string;
	    download_url?: string;
	    info_hash?: string;
	    hash?: string;
	    qualite?: number;
	    saison?: number;
	    episode?: number;
	    full_saison?: boolean;
	    taille?: number;
	    seeders?: number;
	    leechers?: number;
	    completed?: number;
	    author?: string;
	    active?: boolean;
	    created_at?: string;
	    updated_at?: string;
	    qual?: QualDetails;
	    langues_compact?: LangPivot[];
	    subs_compact?: LangPivot[];
	
	    static createFrom(source: any = {}) {
	        return new TorrentItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title_id = source["title_id"];
	        this.torrent_name = source["torrent_name"];
	        this.download_url = source["download_url"];
	        this.info_hash = source["info_hash"];
	        this.hash = source["hash"];
	        this.qualite = source["qualite"];
	        this.saison = source["saison"];
	        this.episode = source["episode"];
	        this.full_saison = source["full_saison"];
	        this.taille = source["taille"];
	        this.seeders = source["seeders"];
	        this.leechers = source["leechers"];
	        this.completed = source["completed"];
	        this.author = source["author"];
	        this.active = source["active"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.qual = this.convertValues(source["qual"], QualDetails);
	        this.langues_compact = this.convertValues(source["langues_compact"], LangPivot);
	        this.subs_compact = this.convertValues(source["subs_compact"], LangPivot);
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
	export class TorrentsResult {
	    torrents: TorrentItem[];
	    count: number;
	    charged: number;
	    already_paid: number;
	
	    static createFrom(source: any = {}) {
	        return new TorrentsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.torrents = this.convertValues(source["torrents"], TorrentItem);
	        this.count = source["count"];
	        this.charged = source["charged"];
	        this.already_paid = source["already_paid"];
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
	export class UploadLienItem {
	    id: number;
	    active: any;
	    lien: string;
	
	    static createFrom(source: any = {}) {
	        return new UploadLienItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.active = source["active"];
	        this.lien = source["lien"];
	    }
	}
	export class UploadLienResult {
	    status: string;
	    liens: UploadLienItem[];
	
	    static createFrom(source: any = {}) {
	        return new UploadLienResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.liens = this.convertValues(source["liens"], UploadLienItem);
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
	export class UploadNzbResult {
	    success: boolean;
	    message: string;
	    // Go type: struct { ID int "json:\"id\""; Active interface {} "json:\"active\"" }
	    nzb: any;
	
	    static createFrom(source: any = {}) {
	        return new UploadNzbResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.nzb = this.convertValues(source["nzb"], Object);
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
	export class UploadTorrentResult {
	    success: boolean;
	    message: string;
	    download_url: string;
	    expires_at: string;
	    // Go type: struct { ID int "json:\"id\""; Hash string "json:\"hash\""; Active bool "json:\"active\"" }
	    torrent: any;
	
	    static createFrom(source: any = {}) {
	        return new UploadTorrentResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.download_url = source["download_url"];
	        this.expires_at = source["expires_at"];
	        this.torrent = this.convertValues(source["torrent"], Object);
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
	export class User {
	    id: number;
	    username: string;
	    email?: string;
	    access_token?: string;
	    avatar?: string;
	    image?: string;
	    created_at?: string;
	    IsPremium?: boolean;
	    is_pro?: boolean;
	    wallet_balance?: string;
	    uploaded?: number;
	    downloaded?: number;
	    ratio?: string;
	    followers_count?: number;
	    followed_users_count?: number;
	    lists_count?: number;
	    api_content_enabled?: boolean;
	    language?: string;
	    country?: string;
	    bio?: string;
	    status?: string;
	    premium_expire?: string;
	    unlimited_until?: string;
	
	    static createFrom(source: any = {}) {
	        return new User(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.username = source["username"];
	        this.email = source["email"];
	        this.access_token = source["access_token"];
	        this.avatar = source["avatar"];
	        this.image = source["image"];
	        this.created_at = source["created_at"];
	        this.IsPremium = source["IsPremium"];
	        this.is_pro = source["is_pro"];
	        this.wallet_balance = source["wallet_balance"];
	        this.uploaded = source["uploaded"];
	        this.downloaded = source["downloaded"];
	        this.ratio = source["ratio"];
	        this.followers_count = source["followers_count"];
	        this.followed_users_count = source["followed_users_count"];
	        this.lists_count = source["lists_count"];
	        this.api_content_enabled = source["api_content_enabled"];
	        this.language = source["language"];
	        this.country = source["country"];
	        this.bio = source["bio"];
	        this.status = source["status"];
	        this.premium_expire = source["premium_expire"];
	        this.unlimited_until = source["unlimited_until"];
	    }
	}

}

export namespace config {
	
	export class Config {
	    hydracker_token: string;
	    tmdb_api_key: string;
	    tmdb_proxy_url: string;
	    one_fichier_api_key: string;
	    sendcm_api_key: string;
	    nexum_api_key: string;
	    nexum_base_url: string;
	    usenet_host: string;
	    usenet_port: number;
	    usenet_ssl: boolean;
	    usenet_user: string;
	    usenet_password: string;
	    usenet_connections: number;
	    usenet_group: string;
	    parpar_redundancy: number;
	    parpar_threads: number;
	    parpar_slice_size: number;
	    ftp_host: string;
	    ftp_port: number;
	    ftp_user: string;
	    ftp_password: string;
	    ftp_path: string;
	    private_ftp_host: string;
	    private_ftp_port: number;
	    private_ftp_user: string;
	    private_ftp_password: string;
	    private_ftp_path: string;
	    seedbox_url: string;
	    seedbox_user: string;
	    seedbox_password: string;
	    seedbox_label: string;
	    private_seedbox_url: string;
	    private_seedbox_user: string;
	    private_seedbox_password: string;
	    private_seedbox_label: string;
	    private_qbit_url: string;
	    private_qbit_user: string;
	    private_qbit_password: string;
	    qbit_url: string;
	    qbit_user: string;
	    qbit_password: string;
	    mod_seedbox_url: string;
	    mod_seedbox_user: string;
	    mod_seedbox_password: string;
	    ftp_mod_host: string;
	    ftp_mod_port: number;
	    ftp_mod_user: string;
	    ftp_mod_password: string;
	    ftp_mod_path: string;
	    nextcloud_admin_url: string;
	    nextcloud_admin_user: string;
	    nextcloud_admin_password: string;
	    nextcloud_admin_path: string;
	    qbit_admin_url: string;
	    qbit_admin_user: string;
	    qbit_admin_password: string;
	    seedbox_settings_password_hash: string;
	    torrent_admin_acknowledged: boolean;
	    tracker_url: string;
	    torrent_piece_size: number;
	    hydracker_base_url: string;
	    lihdl_base_url: string;
	    media_search_url: string;
	    lihdl_user: string;
	    lihdl_password: string;
	    lihdl_settings_password_hash: string;
	    watch_folder: string;
	    watch_auto_start: boolean;
	    proxy_url: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hydracker_token = source["hydracker_token"];
	        this.tmdb_api_key = source["tmdb_api_key"];
	        this.tmdb_proxy_url = source["tmdb_proxy_url"];
	        this.one_fichier_api_key = source["one_fichier_api_key"];
	        this.sendcm_api_key = source["sendcm_api_key"];
	        this.nexum_api_key = source["nexum_api_key"];
	        this.nexum_base_url = source["nexum_base_url"];
	        this.usenet_host = source["usenet_host"];
	        this.usenet_port = source["usenet_port"];
	        this.usenet_ssl = source["usenet_ssl"];
	        this.usenet_user = source["usenet_user"];
	        this.usenet_password = source["usenet_password"];
	        this.usenet_connections = source["usenet_connections"];
	        this.usenet_group = source["usenet_group"];
	        this.parpar_redundancy = source["parpar_redundancy"];
	        this.parpar_threads = source["parpar_threads"];
	        this.parpar_slice_size = source["parpar_slice_size"];
	        this.ftp_host = source["ftp_host"];
	        this.ftp_port = source["ftp_port"];
	        this.ftp_user = source["ftp_user"];
	        this.ftp_password = source["ftp_password"];
	        this.ftp_path = source["ftp_path"];
	        this.private_ftp_host = source["private_ftp_host"];
	        this.private_ftp_port = source["private_ftp_port"];
	        this.private_ftp_user = source["private_ftp_user"];
	        this.private_ftp_password = source["private_ftp_password"];
	        this.private_ftp_path = source["private_ftp_path"];
	        this.seedbox_url = source["seedbox_url"];
	        this.seedbox_user = source["seedbox_user"];
	        this.seedbox_password = source["seedbox_password"];
	        this.seedbox_label = source["seedbox_label"];
	        this.private_seedbox_url = source["private_seedbox_url"];
	        this.private_seedbox_user = source["private_seedbox_user"];
	        this.private_seedbox_password = source["private_seedbox_password"];
	        this.private_seedbox_label = source["private_seedbox_label"];
	        this.private_qbit_url = source["private_qbit_url"];
	        this.private_qbit_user = source["private_qbit_user"];
	        this.private_qbit_password = source["private_qbit_password"];
	        this.qbit_url = source["qbit_url"];
	        this.qbit_user = source["qbit_user"];
	        this.qbit_password = source["qbit_password"];
	        this.mod_seedbox_url = source["mod_seedbox_url"];
	        this.mod_seedbox_user = source["mod_seedbox_user"];
	        this.mod_seedbox_password = source["mod_seedbox_password"];
	        this.ftp_mod_host = source["ftp_mod_host"];
	        this.ftp_mod_port = source["ftp_mod_port"];
	        this.ftp_mod_user = source["ftp_mod_user"];
	        this.ftp_mod_password = source["ftp_mod_password"];
	        this.ftp_mod_path = source["ftp_mod_path"];
	        this.nextcloud_admin_url = source["nextcloud_admin_url"];
	        this.nextcloud_admin_user = source["nextcloud_admin_user"];
	        this.nextcloud_admin_password = source["nextcloud_admin_password"];
	        this.nextcloud_admin_path = source["nextcloud_admin_path"];
	        this.qbit_admin_url = source["qbit_admin_url"];
	        this.qbit_admin_user = source["qbit_admin_user"];
	        this.qbit_admin_password = source["qbit_admin_password"];
	        this.seedbox_settings_password_hash = source["seedbox_settings_password_hash"];
	        this.torrent_admin_acknowledged = source["torrent_admin_acknowledged"];
	        this.tracker_url = source["tracker_url"];
	        this.torrent_piece_size = source["torrent_piece_size"];
	        this.hydracker_base_url = source["hydracker_base_url"];
	        this.lihdl_base_url = source["lihdl_base_url"];
	        this.media_search_url = source["media_search_url"];
	        this.lihdl_user = source["lihdl_user"];
	        this.lihdl_password = source["lihdl_password"];
	        this.lihdl_settings_password_hash = source["lihdl_settings_password_hash"];
	        this.watch_folder = source["watch_folder"];
	        this.watch_auto_start = source["watch_auto_start"];
	        this.proxy_url = source["proxy_url"];
	    }
	}

}

export namespace history {
	
	export class Entry {
	    id: number;
	    timestamp: string;
	    type: string;
	    title_id: number;
	    title_name: string;
	    saison: number;
	    episode: number;
	    qualite: number;
	    qualite_name: string;
	    hydracker_id: number;
	    filename: string;
	    links: string;
	    status: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.timestamp = source["timestamp"];
	        this.type = source["type"];
	        this.title_id = source["title_id"];
	        this.title_name = source["title_name"];
	        this.saison = source["saison"];
	        this.episode = source["episode"];
	        this.qualite = source["qualite"];
	        this.qualite_name = source["qualite_name"];
	        this.hydracker_id = source["hydracker_id"];
	        this.filename = source["filename"];
	        this.links = source["links"];
	        this.status = source["status"];
	        this.error = source["error"];
	    }
	}

}

export namespace main {
	
	export class AuthResult {
	    username: string;
	    role: string;
	    title: string;
	    avatar?: string;
	    badge?: string;
	    color?: string;
	    tabs?: string[];
	    permissions?: string[];
	
	    static createFrom(source: any = {}) {
	        return new AuthResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.role = source["role"];
	        this.title = source["title"];
	        this.avatar = source["avatar"];
	        this.badge = source["badge"];
	        this.color = source["color"];
	        this.tabs = source["tabs"];
	        this.permissions = source["permissions"];
	    }
	}
	export class AutoReseedDDLResult {
	    lien_id: number;
	    filename: string;
	    host: string;
	    size_bytes: number;
	    ftp_remote_name: string;
	
	    static createFrom(source: any = {}) {
	        return new AutoReseedDDLResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lien_id = source["lien_id"];
	        this.filename = source["filename"];
	        this.host = source["host"];
	        this.size_bytes = source["size_bytes"];
	        this.ftp_remote_name = source["ftp_remote_name"];
	    }
	}
	export class AutoReseedFullResult {
	    torrent_id: number;
	    torrent_name: string;
	    expected_filename: string;
	    matched_lien_id: number;
	    matched_host: string;
	    size_bytes: number;
	    info_hash: string;
	    seedbox_path: string;
	    rechecked: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AutoReseedFullResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.torrent_id = source["torrent_id"];
	        this.torrent_name = source["torrent_name"];
	        this.expected_filename = source["expected_filename"];
	        this.matched_lien_id = source["matched_lien_id"];
	        this.matched_host = source["matched_host"];
	        this.size_bytes = source["size_bytes"];
	        this.info_hash = source["info_hash"];
	        this.seedbox_path = source["seedbox_path"];
	        this.rechecked = source["rechecked"];
	    }
	}
	export class AutoReseedResult {
	    torrent_id: number;
	    torrent_name: string;
	    quality: number;
	    lang: number;
	    saison: number;
	    episode: number;
	    size_bytes: number;
	    seedbox_path: string;
	
	    static createFrom(source: any = {}) {
	        return new AutoReseedResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.torrent_id = source["torrent_id"];
	        this.torrent_name = source["torrent_name"];
	        this.quality = source["quality"];
	        this.lang = source["lang"];
	        this.saison = source["saison"];
	        this.episode = source["episode"];
	        this.size_bytes = source["size_bytes"];
	        this.seedbox_path = source["seedbox_path"];
	    }
	}
	export class CheckTorrentEntry {
	    hash: string;
	    name: string;
	    file_name: string;
	    has_error: boolean;
	    message: string;
	    is_active: number;
	    state: number;
	    size: number;
	    done: number;
	    lihdl_url: string;
	    lihdl_name: string;
	
	    static createFrom(source: any = {}) {
	        return new CheckTorrentEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.name = source["name"];
	        this.file_name = source["file_name"];
	        this.has_error = source["has_error"];
	        this.message = source["message"];
	        this.is_active = source["is_active"];
	        this.state = source["state"];
	        this.size = source["size"];
	        this.done = source["done"];
	        this.lihdl_url = source["lihdl_url"];
	        this.lihdl_name = source["lihdl_name"];
	    }
	}
	export class DDLResolved {
	    url: string;
	    filename: string;
	
	    static createFrom(source: any = {}) {
	        return new DDLResolved(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.filename = source["filename"];
	    }
	}
	export class DDLWorkflowResult {
	    links: string[];
	    hydracker_id: number;
	
	    static createFrom(source: any = {}) {
	        return new DDLWorkflowResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.links = source["links"];
	        this.hydracker_id = source["hydracker_id"];
	    }
	}
	export class DeleteTorrentResult {
	    hydracker_ok: boolean;
	    hydracker_err?: string;
	    seedbox_ok: boolean;
	    seedbox_err?: string;
	    used_seedbox: string;
	    ftp_deleted: string[];
	    ftp_errors: string[];
	    files_attempted: string[];
	    used_ftp: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteTorrentResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hydracker_ok = source["hydracker_ok"];
	        this.hydracker_err = source["hydracker_err"];
	        this.seedbox_ok = source["seedbox_ok"];
	        this.seedbox_err = source["seedbox_err"];
	        this.used_seedbox = source["used_seedbox"];
	        this.ftp_deleted = source["ftp_deleted"];
	        this.ftp_errors = source["ftp_errors"];
	        this.files_attempted = source["files_attempted"];
	        this.used_ftp = source["used_ftp"];
	    }
	}
	export class FicheContent {
	    torrents?: api.TorrentsResult;
	    nzbs?: api.NzbsResult;
	    liens?: api.LiensResult;
	    charged_total: number;
	
	    static createFrom(source: any = {}) {
	        return new FicheContent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.torrents = this.convertValues(source["torrents"], api.TorrentsResult);
	        this.nzbs = this.convertValues(source["nzbs"], api.NzbsResult);
	        this.liens = this.convertValues(source["liens"], api.LiensResult);
	        this.charged_total = source["charged_total"];
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
	export class FindHydrackerSourcesResult {
	    liens: api.Lien[];
	    nzbs: api.Nzb[];
	    torrents: api.TorrentItem[];
	
	    static createFrom(source: any = {}) {
	        return new FindHydrackerSourcesResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.liens = this.convertValues(source["liens"], api.Lien);
	        this.nzbs = this.convertValues(source["nzbs"], api.Nzb);
	        this.torrents = this.convertValues(source["torrents"], api.TorrentItem);
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
	export class NzbFileEntry {
	    filename: string;
	    size?: number;
	
	    static createFrom(source: any = {}) {
	        return new NzbFileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filename = source["filename"];
	        this.size = source["size"];
	    }
	}
	export class NzbWorkflowResult {
	    nzb_path: string;
	    hydracker_id: number;
	
	    static createFrom(source: any = {}) {
	        return new NzbWorkflowResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nzb_path = source["nzb_path"];
	        this.hydracker_id = source["hydracker_id"];
	    }
	}
	export class ReseedPrepareResult {
	    torrent_name: string;
	    first_file_name: string;
	    size: number;
	    info_hash: string;
	    search?: mediasearch.SearchResult;
	    hydracker_fiche?: api.PartialTitle;
	
	    static createFrom(source: any = {}) {
	        return new ReseedPrepareResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.torrent_name = source["torrent_name"];
	        this.first_file_name = source["first_file_name"];
	        this.size = source["size"];
	        this.info_hash = source["info_hash"];
	        this.search = this.convertValues(source["search"], mediasearch.SearchResult);
	        this.hydracker_fiche = this.convertValues(source["hydracker_fiche"], api.PartialTitle);
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
	export class RoleDef {
	    badge: string;
	    color: string;
	    title?: string;
	    tabs: string[];
	    permissions?: string[];
	
	    static createFrom(source: any = {}) {
	        return new RoleDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.badge = source["badge"];
	        this.color = source["color"];
	        this.title = source["title"];
	        this.tabs = source["tabs"];
	        this.permissions = source["permissions"];
	    }
	}
	export class TeamUser {
	    pseudo: string;
	    role: string;
	    title?: string;
	    password_hash: string;
	
	    static createFrom(source: any = {}) {
	        return new TeamUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pseudo = source["pseudo"];
	        this.role = source["role"];
	        this.title = source["title"];
	        this.password_hash = source["password_hash"];
	    }
	}
	export class TeamConfig {
	    roles: Record<string, RoleDef>;
	    users: TeamUser[];
	
	    static createFrom(source: any = {}) {
	        return new TeamConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.roles = this.convertValues(source["roles"], RoleDef, true);
	        this.users = this.convertValues(source["users"], TeamUser);
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
	
	export class TorrentWorkflowResult {
	    torrent_path: string;
	    hydracker_id: number;
	    hydracker_torrent_path: string;
	    seedbox_path: string;
	
	    static createFrom(source: any = {}) {
	        return new TorrentWorkflowResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.torrent_path = source["torrent_path"];
	        this.hydracker_id = source["hydracker_id"];
	        this.hydracker_torrent_path = source["hydracker_torrent_path"];
	        this.seedbox_path = source["seedbox_path"];
	    }
	}
	export class UpdateInfo {
	    available: boolean;
	    current: string;
	    latest: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.url = source["url"];
	    }
	}
	export class UploaderRow {
	    author: string;
	    torrents: number;
	    nzbs: number;
	    liens: number;
	    total: number;
	    total_size: number;
	    last_upload_at: string;
	
	    static createFrom(source: any = {}) {
	        return new UploaderRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.author = source["author"];
	        this.torrents = source["torrents"];
	        this.nzbs = source["nzbs"];
	        this.liens = source["liens"];
	        this.total = source["total"];
	        this.total_size = source["total_size"];
	        this.last_upload_at = source["last_upload_at"];
	    }
	}
	export class UploaderScanResult {
	    uploaders: UploaderRow[];
	    scanned_titles: number;
	    // Go type: struct { Torrents int "json:\"torrents\""; Nzbs int "json:\"nzbs\""; Liens int "json:\"liens\"" }
	    scanned_items: any;
	    oldest_scanned: string;
	    newest_scanned: string;
	    duration_sec: number;
	
	    static createFrom(source: any = {}) {
	        return new UploaderScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uploaders = this.convertValues(source["uploaders"], UploaderRow);
	        this.scanned_titles = source["scanned_titles"];
	        this.scanned_items = this.convertValues(source["scanned_items"], Object);
	        this.oldest_scanned = source["oldest_scanned"];
	        this.newest_scanned = source["newest_scanned"];
	        this.duration_sec = source["duration_sec"];
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

export namespace mediasearch {
	
	export class SearchResult {
	    tmdb_id: number;
	    media_type: string;
	    title_fr: string;
	    title_vo: string;
	    year: string;
	    poster_url: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tmdb_id = source["tmdb_id"];
	        this.media_type = source["media_type"];
	        this.title_fr = source["title_fr"];
	        this.title_vo = source["title_vo"];
	        this.year = source["year"];
	        this.poster_url = source["poster_url"];
	    }
	}

}

export namespace parser {
	
	export class FileInfo {
	    title: string;
	    year: string;
	    quality: string;
	    source: string;
	    video_codec: string;
	    audio_codec: string;
	    languages: string[];
	    group: string;
	    season: number;
	    episode: number;
	    raw: string;
	
	    static createFrom(source: any = {}) {
	        return new FileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.year = source["year"];
	        this.quality = source["quality"];
	        this.source = source["source"];
	        this.video_codec = source["video_codec"];
	        this.audio_codec = source["audio_codec"];
	        this.languages = source["languages"];
	        this.group = source["group"];
	        this.season = source["season"];
	        this.episode = source["episode"];
	        this.raw = source["raw"];
	    }
	}

}

export namespace tester {
	
	export class Result {
	    ok: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	    }
	}

}

export namespace tmdb {
	
	export class Movie {
	    id: number;
	    title: string;
	    original_title: string;
	    name: string;
	    original_name: string;
	    overview: string;
	    poster_path: string;
	    release_date: string;
	    first_air_date: string;
	    vote_average: number;
	    media_type: string;
	    imdb_id?: string;
	    note_imdb?: number;
	    vote_imdb?: number;
	
	    static createFrom(source: any = {}) {
	        return new Movie(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.original_title = source["original_title"];
	        this.name = source["name"];
	        this.original_name = source["original_name"];
	        this.overview = source["overview"];
	        this.poster_path = source["poster_path"];
	        this.release_date = source["release_date"];
	        this.first_air_date = source["first_air_date"];
	        this.vote_average = source["vote_average"];
	        this.media_type = source["media_type"];
	        this.imdb_id = source["imdb_id"];
	        this.note_imdb = source["note_imdb"];
	        this.vote_imdb = source["vote_imdb"];
	    }
	}

}

