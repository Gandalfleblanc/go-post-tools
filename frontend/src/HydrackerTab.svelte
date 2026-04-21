<script>
  import { onMount, onDestroy } from 'svelte'
  import { EventsOn, EventsOff, OnFileDrop, OnFileDropOff } from '../wailsjs/runtime/runtime.js'
  import { ParseFilename, TMDBSearch, TMDBGetByID, HydrackerSearch, HydrackerGetByTmdbID, HydrackerGetByID, OpenBrowser, OpenHydrackerAdmin, SelectMkvFile, SelectMkvFiles, PostTorrentWorkflow, PostExistingTorrent, PostNzbWorkflow, PostDDLWorkflow, FetchImageBase64, GetMetaQualities, GetFileSize, ReadFileChunk, MediaSearch, CancelAllWorkflows, Notify, CancelDDLHost, SkipCurrentEpisode } from '../wailsjs/go/main/App.js'
  import { addLog } from './logs.js'
  import { LANGUAGES as HYD_LANGUAGES, SUBS as HYD_SUBS } from './hydrackerData.js'

  // --- State ---
  let dragOver = false
  let file = null
  let fileInfo = null
  let mediaInfo = null
  let mediaInfoLoading = false
  let mediaInfoError = ''

  // TMDB
  let tmdbResults = []
  let tmdbAmbiguous = false
  let selectedTMDB = null
  let tmdbSearchQuery = ''
  let tmdbSearchId = ''
  let tmdbSearchLoading = false

  // Poster TMDB
  let posterDataUrl = ''
  // Poster Hydracker
  let hydrackerPosterUrl = ''

  // Hydracker
  let hydrackerResults = []
  let hydrackerSearchCache = []   // derniers résultats pour restaurer après désélection
  let hydrackerSearchQuery = ''
  let hydrackerSearchLoading = false
  let selectedHydracker = null
  let hydrackerNotFound = false      // fiche introuvable après recherche auto
  let hydrackerManualId = ''         // saisie manuelle de l'ID Hydracker
  let hydrackerManualLoading = false

  // Hydracker post fields
  let postQuality = 0
  let postLanguages = []   // [{id, name}]
  let postSubs = []        // [{id, name}]
  let langsAutoFilled = false
  let subsAutoFilled = false
  let postUploadTypes = { nzb: false, torrent: true, ddl: false }
  let postDdlHosts = { onefichier: true, sendcm: true }
  let postSeason = 0
  let postEpisode = 0

  // Fichiers sélectionnés pour l'upload
  let mkvFilePath = ''
  // Mode "torrent existant" (depuis Reseed) : on poste le .torrent tel quel à Hydracker
  // (pas de FTP, pas de seedbox, pas de regénération depuis MKV)
  let existingTorrentPath = ''
  let mediaInfoOpen = true   // MediaInfo dépliée par défaut
  let nfoOpen = false        // NFO repliée par défaut
  let recapOpen = true       // Récapitulatif déplié par défaut

  // Résultat du post
  let postLoading = false
  let postResult = null  // { ok: bool, message: string, details: string }

  // Progression NZB en temps réel
  let nzbStatus = ''
  let nzbParparPct = 0
  let nzbNyuuPct = 0
  let nzbNyuuSpeed = ''
  let nzbNyuuETA = ''
  let nzbNyuuArticles = ''

  // Progression Torrent
  let torrentState = {
    stage: '',       // 'ftp' | 'create' | 'post' | 'download' | 'seedbox' | 'done'
    msg: '',
    ftpPct: 0, ftpSpeed: 0,
    createPct: 0,
    seedboxPct: 0, seedboxSpeed: 0,
  }

  // Progression DDL — une entrée par hôte
  let ddlHosts = {
    '1Fichier': { active: false, filename: '', pct: 0, speed: '', done: false, posting: false, posted: false, hydrackerID: 0, error: '' },
    'Send.now':  { active: false, filename: '', pct: 0, speed: '', done: false, posting: false, posted: false, hydrackerID: 0, error: '' },
  }

  // Meta depuis l'API Hydracker
  let qualityOptions = []  // [{id, name}]
  let langOptions = []     // [{id, name}]
  let subOptions = []      // [{id, name}] — liste spécifique aux sous-titres

  onMount(async () => {
    try { qualityOptions = await GetMetaQualities() || [] } catch(e) { console.error(e) }
    // Listes statiques extraites du bootstrap Hydracker (audio et subs sont distinctes)
    langOptions = HYD_LANGUAGES
    subOptions = HYD_SUBS
    EventsOn('nzb:status',  s  => { nzbStatus = s })
    EventsOn('nzb:parpar',  p  => { if (p.percent !== undefined) nzbParparPct = p.percent })
    EventsOn('nzb:nyuu',    p  => {
      if (p.percent   !== undefined) nzbNyuuPct      = p.percent
      if (p.speed     !== undefined) nzbNyuuSpeed    = p.speed
      if (p.eta       !== undefined) nzbNyuuETA      = p.eta
      if (p.articles  !== undefined) nzbNyuuArticles = p.articles
    })
    EventsOn('ddl:progress', p => {
      if (!p.host) return
      ddlHosts = { ...ddlHosts, [p.host]: { ...ddlHosts[p.host], active: true, filename: p.filename || ddlHosts[p.host]?.filename || '', pct: p.percent ?? 0, speed: p.speed || '' } }
    })
    EventsOn('ddl:done', p => {
      if (!p.host) return
      ddlHosts = { ...ddlHosts, [p.host]: { ...ddlHosts[p.host], done: !p.error, error: p.error || '', active: false, pct: p.error ? ddlHosts[p.host]?.pct : 100 } }
    })
    EventsOn('ddl:host-skipped', host => {
      if (!host) return
      ddlHosts = { ...ddlHosts, [host]: { ...ddlHosts[host], skipped: true, active: false, error: 'skippé par utilisateur' } }
      // Désactive ce host pour les épisodes suivants de la queue
      if (host === '1Fichier') postDdlHosts = { ...postDdlHosts, onefichier: false }
      else if (host === 'Send.now') postDdlHosts = { ...postDdlHosts, sendcm: false }
      addLog('DDL', `⏭ ${host} skippé — désactivé pour la suite de la queue`)
    })
    EventsOn('torrent:status', p => {
      torrentState = { ...torrentState, stage: p.stage || '', msg: p.msg || '' }
      addLog('TOR', p.msg || p.stage)
    })
    EventsOn('torrent:ftp',    p => { torrentState = { ...torrentState, ftpPct: p.percent ?? 0, ftpSpeed: p.speed_mb ?? 0 } })
    EventsOn('torrent:create', p => { torrentState = { ...torrentState, createPct: p.percent ?? 0 } })
    EventsOn('torrent:seedbox',p => { torrentState = { ...torrentState, seedboxPct: p.percent ?? 0, seedboxSpeed: p.speed_mb ?? 0 } })
    EventsOn('ddl:posting', p => {
      if (!p.host) return
      const id = p.id || 0
      ddlHosts = { ...ddlHosts, [p.host]: {
        ...ddlHosts[p.host],
        posting: !!p.posting,
        posted: p.posted ? true : ddlHosts[p.host]?.posted,
        hydrackerID: id || ddlHosts[p.host]?.hydrackerID
      } }
    })
    OnFileDrop((_x, _y, paths) => {
      addLog('QUEUE', `OnFileDrop : ${paths?.length || 0} chemin(s) reçu(s)`)
      if (!paths?.length) return
      const valid = paths.filter(p => /\.(mkv|mp4)$/i.test(p))
      addLog('QUEUE', `${valid.length} valide(s) (mkv/mp4)`)
      if (valid.length === 0) return
      if (valid.length === 1 && queue.length === 0 && !queueProcessing && !file) {
        loadFileFromPath(valid[0], null)
      } else {
        valid.forEach(p => enqueue(p))
      }
    }, true)
    window.addEventListener('watch:newfile', onWatchNewFile)
    window.addEventListener('hydracker:preload-torrent', onPreloadTorrent)
    window.addEventListener('keydown', onKeydown)
  })

  // Raccourcis clavier globaux (hors champs texte)
  function onKeydown(e) {
    const tag = (e.target?.tagName || '').toLowerCase()
    const inField = tag === 'input' || tag === 'textarea' || tag === 'select'
    const mod = e.metaKey || e.ctrlKey

    // Cmd+K : focus recherche Hydracker (même depuis un input)
    if (mod && e.key === 'k') {
      e.preventDefault()
      const el = /** @type {HTMLElement|null} */ (document.querySelector('.hyd-search-input'))
      el?.focus()
      return
    }
    // Esc : stop si post en cours
    if (e.key === 'Escape' && (postLoading || queueProcessing)) {
      e.preventDefault()
      stopPost()
      return
    }
    if (inField) return  // les raccourcis suivants ne s'appliquent pas dans un champ
    // Cmd+Entrée : Lancer
    if (mod && e.key === 'Enter') {
      e.preventDefault()
      if (queue.length > 0) processQueue()
      else if (selectedHydracker && postQuality && postLanguages.length) lancerPost()
      return
    }
    // Cmd+. : Stop
    if (mod && e.key === '.') {
      e.preventDefault()
      if (postLoading || queueProcessing) stopPost()
      return
    }
    // Cmd+Backspace : Reset
    if (mod && e.key === 'Backspace') {
      e.preventDefault()
      const btn = /** @type {HTMLElement|null} */ (document.querySelector('.btn-reset'))
      btn?.click()
      return
    }
  }

  // Quand Reseed envoie un .torrent existant à poster ici (sans FTP/seedbox)
  function onPreloadTorrent(ev) {
    const d = ev?.detail
    if (!d?.torrentPath) return
    existingTorrentPath = d.torrentPath
    file = { name: d.torrentName || d.torrentPath.split(/[\\/]/).pop() }
    if (d.hydrackerFiche) {
      selectedHydracker = d.hydrackerFiche
      if (d.hydrackerFiche.poster) {
        FetchImageBase64(d.hydrackerFiche.poster).then(u => hydrackerPosterUrl = u).catch(() => {})
      }
    }
    // Force type Torrent uniquement (pas de DDL/NZB puisqu'on n'a pas de MKV)
    postUploadTypes = { torrent: true, nzb: false, ddl: false }
    addLog('TOR', `📂 .torrent existant chargé depuis Reseed — ${file.name}`)
  }

  function onWatchNewFile(ev) {
    const path = ev?.detail
    if (!path) return
    enqueue(path)
  }

  // --- Queue batch ---
  let queue = []                // chemins en attente
  let queueProcessing = false
  let queueCurrent = ''
  let queueTotal = 0
  let queueDone = 0
  let queueResults = []         // [{ok, filename, message}] cumulés sur la queue

  function enqueue(path) {
    // Insère en triant par nom de fichier (ordre naturel : S01E01, S01E02, S01E03…)
    queue = [...queue, path].sort((a, b) =>
      a.split('/').pop().localeCompare(b.split('/').pop(), undefined, { numeric: true })
    )
    queueTotal = queueDone + queue.length + (queueProcessing ? 1 : 0)
    addLog('QUEUE', `+ ${path.split('/').pop()} (${queue.length} en attente) — clique sur ▶ Lancer pour démarrer`)
    // Preview : si rien n'est chargé, on affiche le 1er fichier de la queue triée
    if (!file && queue.length >= 1) {
      loadFileFromPath(queue[0], null)
    }
  }
  function dequeueAt(idx) {
    queue = queue.filter((_, i) => i !== idx)
    queueTotal = queueDone + queue.length + (queueProcessing ? 1 : 0)
  }
  function clearQueue() {
    queue = []
    queueTotal = queueDone + (queueProcessing ? 1 : 0)
  }

  async function processQueue() {
    if (queueProcessing || queue.length === 0) return
    queueProcessing = true
    queueResults = []
    while (queue.length > 0) {
      const path = queue.shift()
      queue = queue
      queueCurrent = path
      const fname = path.split('/').pop()
      addLog('QUEUE', `▶ ${fname}`)
      try {
        loadFileFromPath(path, null)
        await waitForReady(60000)
        await lancerPost()
        queueDone++
        if (postResult) queueResults = [...queueResults, { ok: postResult.ok, filename: fname, message: postResult.message }]
      } catch(e) {
        addLog('QUEUE', `✗ ${fname} : ${e}`)
        queueResults = [...queueResults, { ok: false, filename: fname, message: String(e) }]
      }
    }
    queueCurrent = ''
    queueProcessing = false
    // Récap final cumulé
    if (queueResults.length > 1) {
      const okCount = queueResults.filter(r => r.ok).length
      const koCount = queueResults.length - okCount
      const lines = queueResults.map(r => `${r.ok ? '✓' : '✗'} ${r.filename}\n  ${r.message}`).join('\n')
      postResult = {
        ok: koCount === 0,
        message: `Queue terminée — ${okCount}/${queueResults.length} OK${koCount > 0 ? ` · ${koCount} erreur${koCount > 1 ? 's' : ''}` : ''}`,
        details: lines,
      }
      try { Notify('Go Post Tools — Queue terminée', `${okCount}/${queueResults.length} OK${koCount > 0 ? ` · ${koCount} erreur${koCount > 1 ? 's' : ''}` : ''}`) } catch(e) {}
    } else if (queueResults.length === 1) {
      const r = queueResults[0]
      try { Notify(r.ok ? '✓ Post terminé' : '✗ Post échoué', r.filename) } catch(e) {}
    }
  }

  function waitForReady(timeoutMs) {
    return new Promise((resolve, reject) => {
      const start = Date.now()
      const tick = () => {
        if (queueCancelled) return reject(new Error('queue annulée'))
        if (selectedHydracker && postQuality && postLanguages.length && mkvFilePath) return resolve()
        // Si action user requise (ambiguïté TMDB ou fiche Hydracker manquante) : pause infinie
        const userActionNeeded = tmdbAmbiguous || hydrackerNotFound
        if (!userActionNeeded && Date.now() - start > timeoutMs) return reject(new Error('timeout chargement fiche'))
        setTimeout(tick, 250)
      }
      tick()
    })
  }
  let queueCancelled = false

  onDestroy(() => {
    EventsOff('nzb:status', 'nzb:parpar', 'nzb:nyuu', 'ddl:progress', 'ddl:done', 'ddl:posting',
              'torrent:status', 'torrent:ftp', 'torrent:create', 'torrent:seedbox')
    OnFileDropOff()
    window.removeEventListener('watch:newfile', onWatchNewFile)
    window.removeEventListener('hydracker:preload-torrent', onPreloadTorrent)
    window.removeEventListener('keydown', onKeydown)
  })

  // Table ISO 639-1/2 + tags de release → nom Hydracker
  const LANG_MAP = {
    // Français
    'fre':'TrueFrench','fra':'TrueFrench','fr':'TrueFrench',
    'french':'TrueFrench','truefrench':'TrueFrench','vf':'TrueFrench','vff':'TrueFrench','vof':'TrueFrench',
    'french (canada)':'French (Canada)',
    // VO / VO sous-titré
    'vo':'VO','voa':'VO','vost':'VO','vostfr':'VO',
    // Anglais
    'eng':'English','en':'English','english':'English',
    // Allemand
    'ger':'German','deu':'German','de':'German','german':'German',
    // Espagnol
    'spa':'Spanish','es':'Spanish','spanish':'Spanish',
    // Italien
    'ita':'Italian','it':'Italian','italian':'Italian',
    // Japonais
    'jpn':'Japanese','ja':'Japanese','japanese':'Japanese',
    // Chinois
    'chi':'Chinese','zho':'Chinese','zh':'Chinese','chinese':'Chinese',
    // Portugais
    'por':'Portuguese','pt':'Portuguese','portuguese':'Portuguese',
    // Russe
    'rus':'Russian','ru':'Russian','russian':'Russian',
    // Coréen
    'kor':'Korean','ko':'Korean','korean':'Korean',
    // Arabe
    'ara':'Arab','ar':'Arab','arab':'Arab','arabic':'Arab',
    // Néerlandais
    'nld':'Dutch','dut':'Dutch','nl':'Dutch','dutch':'Dutch',
    // Polonais
    'pol':'Polish','pl':'Polish','polish':'Polish',
    // Turc
    'tur':'Turkish','tr':'Turkish','turkish':'Turkish',
    // Suédois
    'swe':'Swedish','sv':'Swedish','swedish':'Swedish',
    // Norvégien
    'nor':'Norwegian','no':'Norwegian','norwegian':'Norwegian',
    // Danois
    'dan':'Danish','da':'Danish','danish':'Danish',
    // Finnois
    'fin':'Finnish','fi':'Finnish','finnish':'Finnish',
    // Hongrois
    'hun':'Hungarian','hu':'Hungarian','hungarian':'Hungarian',
    // Tchèque
    'cze':'Czech','ces':'Czech','cs':'Czech','czech':'Czech',
    // Roumain
    'rum':'Romanian','ron':'Romanian','ro':'Romanian','romanian':'Romanian',
    // Grec
    'gre':'Greek','ell':'Greek','el':'Greek','greek':'Greek',
    // Hébreu
    'heb':'Hebrew','he':'Hebrew','hebrew':'Hebrew',
    // Hindi
    'hin':'Hindi','hi':'Hindi','hindi':'Hindi',
    // Thaï
    'tha':'Thai','th':'Thai','thai':'Thai',
    // Ukrainien
    'ukr':'Ukrainian','uk':'Ukrainian','ukrainian':'Ukrainian',
    // Bulgare
    'bul':'Bulgarian','bg':'Bulgarian','bulgarian':'Bulgarian',
    // Croate
    'hrv':'Croatian','hr':'Croatian','croatian':'Croatian',
    // Serbe
    'srp':'Serbian','sr':'Serbian','serbian':'Serbian',
    // Slovaque
    'slk':'Slovak','sk':'Slovak','slovak':'Slovak',
    // Slovène
    'slv':'Slovenian','sl':'Slovenian','slovenian':'Slovenian',
    // Albanais
    'alb':'Albanian','sqi':'Albanian','sq':'Albanian','albanian':'Albanian',
    // Lituanien
    'lit':'Lithuanian','lt':'Lithuanian','lithuanian':'Lithuanian',
    // Letton
    'lav':'Latvian','lv':'Latvian','latvian':'Latvian',
    // Estonien
    'est':'Estonian','et':'Estonian','estonian':'Estonian',
    // Persan
    'per':'Persian','fas':'Persian','fa':'Persian','persian':'Persian',
    // Géorgien
    'geo':'Georgian','kat':'Georgian','ka':'Georgian','georgian':'Georgian',
    // Islandais
    'ice':'Icelandic','isl':'Icelandic','is':'Icelandic','icelandic':'Icelandic',
    // Mongol
    'mon':'Mongolian','mn':'Mongolian','mongolian':'Mongolian',
    // Kazakh
    'kaz':'Kazakh','kk':'Kazakh','kazakh':'Kazakh',
    // Muet
    'mue':'Muet','muet':'Muet',
  }

  function matchLang(raw) {
    const key = (raw || '').toLowerCase().trim()
    const mapped = LANG_MAP[key] || raw
    const target = mapped.toLowerCase()
    return langOptions.find(o => o.name.toLowerCase() === target)
      || langOptions.find(o => o.name.toLowerCase().includes(target) || target.includes(o.name.toLowerCase()))
      || { id: 0, name: mapped }
  }

  // Pour les subs : mappe les variantes audio (TrueFrench, VO…) vers la langue pure.
  function matchSub(raw) {
    const key = (raw || '').toLowerCase().trim()
    let mapped = LANG_MAP[key] || raw
    // Les valeurs audio-only ne sont pas valides en sub → on "dégraise" vers la langue pure.
    const audioToPure = {
      'TrueFrench': 'French',
      'French (Canada)': 'French',
      'VO': 'English',
      'Muet': null,
    }
    if (mapped in audioToPure) {
      const pure = audioToPure[mapped]
      if (!pure) return { id: 0, name: mapped }
      mapped = pure
    }
    const target = mapped.toLowerCase()
    return langOptions.find(o => o.name.toLowerCase() === target)
      || { id: 0, name: mapped }
  }
  function dedupeById(arr) {
    const seen = new Set()
    return arr.filter(x => { const k = x.id !== 0 ? x.id : x.name; if (seen.has(k)) return false; seen.add(k); return true })
  }

  // Variables pour les selects d'ajout
  let langSelectValue = null
  let subSelectValue = null

  // Auto-remplissage réactif dès que API + fichier prêts
  $: if (qualityOptions.length && file?.name && postQuality === 0) {
    const name = file.name.toLowerCase()
    const bitrate = parseInt(String(mediaInfo?.bitrate || '').replace(/[^0-9]/g, '')) || 0
    const isH265 = /\b(x265|h\.?265|hevc)\b/i.test(file.name)
    // Cherche un quality par mots-clés présents dans le nom (insensible à la casse/espaces)
    const findQual = (...kw) => {
      const lc = kw.map(k => k.toLowerCase().replace(/[\s-]/g, ''))
      const o = qualityOptions.find(o => {
        const n = o.name.toLowerCase().replace(/[\s-]/g, '')
        return lc.every(k => n.includes(k))
      })
      return o?.id || 0
    }
    let qualID = 0
    const is4KLight = /4klight/i.test(file.name)
    const is2160p = /\b2160p\b/i.test(file.name)
    const has1080pHDLight = /1080p[\s._-]*hdlight|hdlight[\s._-]*1080p/i.test(file.name)

    // Règles personnalisées (ordre de priorité)
    if (/-xander(\.(mkv|mp4))?$/i.test(file.name)) {
      // -XANDER → toujours ULTRA HDLight x265
      qualID = findQual('ultra', 'hdlight', 'x265') || 60
    } else if (is4KLight || (is2160p && bitrate > 0 && bitrate < 8000)) {
      // 4KLight OU (2160p + bitrate<8000) → ULTRA HDLight
      qualID = findQual('ultra', 'hdlight', 'x265') || 60
    } else if (has1080pHDLight && isH265) {
      // 1080p.HDLight + H265 → HDLight 1080p x265 systématique
      qualID = findQual('hdlight', '1080p', 'x265') || findQual('hdlight', 'x265') || 50
    } else if (/\bweb([-.]?(?:rip|dl))?\b/.test(name) || name.includes('.web.')) {
      if (isH265) qualID = findQual('web', 'x265') || findQual('webrip', 'x265')
      if (!qualID) qualID = (bitrate > 0 && bitrate <= 3000) ? 94 : 4 // WEB 1080p Light sinon WEB
    } else if (/\bblu-?ray\b/.test(name)) {
      if (isH265) qualID = findQual('bluray', 'x265')
      if (!qualID && bitrate > 0 && bitrate <= 3000) qualID = 50 // HDLight 1080p
    } else if (/hdlight/i.test(name)) {
      if (isH265) qualID = findQual('hdlight', 'x265')
      if (!qualID) qualID = 50 // HDLight 1080p (fallback)
    }

    if (qualID) {
      postQuality = qualID
    } else if (fileInfo?.quality) {
      // Fallback : détection classique par le parser
      const qual = fileInfo.quality.toLowerCase()
      const src  = (fileInfo.source || '').toLowerCase()
      let q = src ? qualityOptions.find(o => { const n = o.name.toLowerCase(); return n.includes(qual) && n.includes(src) }) : null
      if (!q) q = qualityOptions.find(o => o.name.toLowerCase().includes(qual))
      if (q) postQuality = q.id
    }
  }

  // Auto-détection des langues audio : mediaInfo est prioritaire (plus fiable que le parser)
  // Si le parser a détecté "MULTi" / "MULTI" / "DUAL" on l'ignore (c'est un tag, pas une langue)
  $: if (langOptions.length && !langsAutoFilled) {
    const audioTracks = (mediaInfo?.audios || []).map(a => a.lang).filter(Boolean)
    if (audioTracks.length) {
      postLanguages = dedupeById(mapAudioTracks(audioTracks))
      langsAutoFilled = true
    } else if (fileInfo?.languages?.length) {
      // Fallback sur le parser, sans les tags génériques
      const clean = fileInfo.languages.filter(l => !['multi','multil','dual','vff'].includes(l.toLowerCase()))
      if (clean.length) { postLanguages = dedupeById(clean.map(matchLang)); langsAutoFilled = true }
    }
  }

  $: if (langOptions.length && !subsAutoFilled && mediaInfo?.subs?.length) {
    postSubs = dedupeById(mediaInfo.subs.map(matchSub))
    subsAutoFilled = true
  }

  // Mappe une liste de codes ISO audio vers les noms Hydracker, en gérant les pistes multiples.
  // ex: ['fr','fr','en'] → [TrueFrench, French (Canada), English]
  function mapAudioTracks(codes) {
    const frenchOrder = ['TrueFrench', 'French (Canada)', 'FRENCH AD']
    const result = []
    let frIdx = 0
    for (const code of codes) {
      const c = (code || '').toLowerCase()
      if (c.startsWith('fr')) {
        const name = frenchOrder[frIdx] || 'TrueFrench'
        frIdx++
        const found = langOptions.find(o => o.name === name)
        if (found) result.push(found)
      } else {
        result.push(matchLang(code))
      }
    }
    return result
  }

  // --- Drop zone ---
  // OnFileDrop (Wails) est le seul handler — le ondrop HTML empêche Wails de voir l'événement OS
  function onDragOver(e) { e.preventDefault(); dragOver = true }
  function onDragLeave() { dragOver = false }

  async function onFileInput(e) {
    const f = e.target.files[0]
    if (f) await loadFileFromPath(null, f.name)  // parcourir : pas de chemin OS, juste le nom
  }

  async function loadFileFromPath(path, name) {
    const filename = name || path.split(/[\\/]/).pop()
    if (path) mkvFilePath = path
    mediaInfo = null
    selectedTMDB = null
    selectedHydracker = null
    tmdbResults = []
    hydrackerResults = []
    hydrackerSearchCache = []
    posterDataUrl = ''
    hydrackerPosterUrl = ''
    hydrackerNotFound = false
    hydrackerManualId = ''
    postQuality = 0
    postLanguages = []
    postSubs = []
    postSeason = 0
    postEpisode = 0
    langsAutoFilled = false
    subsAutoFilled = false

    // Objet file synthétique pour afficher le nom
    file = { name: filename }

    // 1. Parser le nom de fichier
    fileInfo = await ParseFilename(filename)
    addLog('TMDB', `parse : title="${fileInfo?.title || ''}" year="${fileInfo?.year || ''}" S${fileInfo?.season || 0}E${fileInfo?.episode || 0}`)
    if (fileInfo?.season) postSeason = fileInfo.season
    if (fileInfo?.episode) postEpisode = fileInfo.episode

    // 2. MediaInfo via Go (chemin filesystem réel)
    if (path) await analyzeMediaInfoFromPath(path)

    // 3. Recherche TMDB automatique
    if (fileInfo?.title) {
      await autoSearchTMDB(fileInfo.title)
    } else {
      addLog('TMDB', '⚠ pas de titre extrait du nom de fichier — recherche auto annulée')
    }
  }

  // --- MediaInfo via Go filesystem ---
  async function analyzeMediaInfoFromPath(path) {
    mediaInfoLoading = true
    mediaInfoError = ''
    try {
      addLog('MI', `analyse : ${path.split('/').pop()}`)
      const MediaInfo = (await import('mediainfo.js')).default
      const mi = await MediaInfo({ format: 'object', locateFile: () => '/MediaInfoModule.wasm' })
      const fileSize = await GetFileSize(path)
      const getSize = () => fileSize
      const toU8 = (bytes) => {
        if (bytes instanceof Uint8Array) return bytes
        if (Array.isArray(bytes)) return new Uint8Array(bytes)
        if (typeof bytes === 'string') {
          const bin = atob(bytes)
          const u8 = new Uint8Array(bin.length)
          for (let i = 0; i < bin.length; i++) u8[i] = bin.charCodeAt(i)
          return u8
        }
        return new Uint8Array(0)
      }
      const readChunk = async (size, offset) => toU8(await ReadFileChunk(path, offset, size))
      const result = await mi.analyzeData(getSize, readChunk)
      mediaInfo = parseMediaInfo(result)
      if (!mediaInfo || (!mediaInfo.filesize && !mediaInfo.videoCodec && !mediaInfo.duration)) {
        mediaInfoError = 'Aucune donnée exploitable'
        mediaInfo = null
        addLog('MI', `⚠ ${mediaInfoError}`)
      } else {
        addLog('MI', `✓ ${mediaInfo.videoCodec || '?'} · ${mediaInfo.audios?.length || 0} audio · ${mediaInfo.subs?.length || 0} subs`)
      }
      mi.close()
    } catch(e) {
      const msg = String(e?.message || e || 'erreur inconnue')
      mediaInfoError = msg
      addLog('MI', `✗ erreur : ${msg}`)
    }
    mediaInfoLoading = false
  }

  function parseMediaInfo(result) {
    const tracks = result?.media?.track || []
    const general = tracks.find(t => t['@type'] === 'General') || {}
    const video = tracks.find(t => t['@type'] === 'Video') || {}
    const audios = tracks.filter(t => t['@type'] === 'Audio')
    const texts = tracks.filter(t => t['@type'] === 'Text')

    return {
      duration: general.Duration ? formatDuration(parseFloat(general.Duration)) : null,
      filesize: general.FileSize ? formatSize(parseInt(general.FileSize)) : null,
      videoCodec: video.Format || video.CodecID || null,
      videoProfile: video.Format_Profile || null,
      width: video.Width || null,
      height: video.Height || null,
      bitrate: video.BitRate ? Math.round(parseInt(video.BitRate) / 1000) + ' kbps' : null,
      audios: audios.map(a => ({
        codec: a.Format || a.CodecID || '?',
        channels: a.Channels ? a.Channels + 'ch' : null,
        lang: a.Language || null,
      })),
      langs: [...new Set(audios.map(a => a.Language).filter(Boolean))],
      subs: texts.map(t => t.Language || t.Title).filter(Boolean),
    }
  }

  function formatDuration(s) {
    const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = Math.floor(s % 60)
    return h > 0 ? `${h}h${String(m).padStart(2,'0')}m` : `${m}m${String(sec).padStart(2,'0')}s`
  }
  function formatSize(b) {
    if (b > 1e9) return (b/1e9).toFixed(2) + ' GB'
    return (b/1e6).toFixed(0) + ' MB'
  }

  // --- TMDB ---
  async function autoSearchTMDB(query) {
    tmdbSearchLoading = true
    addLog('TMDB', `🔍 Recherche auto : "${query}"${fileInfo?.year ? ' (' + fileInfo.year + ')' : ''}`)
    try {
      // Recherche via mediasearch (plus fiable, inclut le tmdb_id directement)
      const year = fileInfo?.year || ''
      const q = year ? `${query} ${year}` : query
      let spResults = []
      try { spResults = await MediaSearch(q) || [] } catch(e) { addLog('TMDB', '⚠ mediasearch: ' + e) }
      addLog('TMDB', `mediasearch : ${spResults.length} résultat(s)`)

      if (spResults.length === 0) {
        // Fallback sur TMDB direct si mediasearch ne trouve rien
        try { tmdbResults = await TMDBSearch(query) || [] } catch(e) { addLog('TMDB', '✗ TMDB API : ' + e); tmdbResults = [] }
        addLog('TMDB', `TMDB API : ${tmdbResults.length} résultat(s)`)
      } else {
        // Convertit les résultats mediasearch en objets compatibles TMDB pour la modal
        tmdbResults = spResults.map(r => ({
          id: r.tmdb_id,
          media_type: r.media_type,
          title: r.title_fr || r.title_vo,
          name: r.title_fr || r.title_vo,
          release_date: r.year ? r.year + '-01-01' : '',
          first_air_date: r.year ? r.year + '-01-01' : '',
          poster_path: '',
          _poster_full: r.poster_url,
          _from_mediasearch: true,
        }))
      }

      if (tmdbResults.length === 1) {
        await selectTMDB(tmdbResults[0])
      } else if (tmdbResults.length > 1) {
        tmdbAmbiguous = true
      }
    } catch(e) { console.error(e) }
    tmdbSearchLoading = false
  }

  async function manualTMDBSearch() {
    tmdbSearchLoading = true
    tmdbAmbiguous = false
    try {
      if (tmdbSearchId) {
        const movie = await TMDBGetByID(parseInt(tmdbSearchId), 'movie')
        tmdbResults = movie ? [movie] : []
      } else if (tmdbSearchQuery) {
        tmdbResults = await TMDBSearch(tmdbSearchQuery) || []
      }
      if (tmdbResults.length === 1) selectTMDB(tmdbResults[0])
      else if (tmdbResults.length > 1) tmdbAmbiguous = true
    } catch(e) { console.error(e) }
    tmdbSearchLoading = false
  }

  async function selectTMDB(movie) {
    // Si le movie vient de mediasearch, on enrichit avec TMDB pour avoir synopsis + poster_path officiel
    if (movie._from_mediasearch && movie.id) {
      try {
        const full = await TMDBGetByID(movie.id, movie.media_type || 'movie')
        if (full) { movie = { ...movie, ...full } }
      } catch(e) { console.error('enrich TMDB:', e) }
    }
    selectedTMDB = movie
    tmdbAmbiguous = false
    posterDataUrl = ''
    selectedHydracker = null
    hydrackerPosterUrl = ''
    if (movie.poster_path) {
      try {
        posterDataUrl = await FetchImageBase64('https://image.tmdb.org/t/p/w342' + movie.poster_path)
      } catch(e) { console.error('poster fetch error:', e) }
    } else if (movie._poster_full) {
      try { posterDataUrl = await FetchImageBase64(movie._poster_full) } catch(e) {}
    }
    // Auto-recherche Hydracker par ID TMDB
    hydrackerNotFound = false
    hydrackerManualId = ''
    if (movie.id) {
      try {
        const found = await HydrackerGetByTmdbID(movie.id)
        if (found) {
          await selectHydracker(found)
        } else {
          // Fallback : recherche par titre
          const title = movie.title || movie.name
          let matched = false
          if (title) {
            const results = await HydrackerSearch(title) || []
            hydrackerSearchCache = results
            if (results.length === 1) {
              await selectHydracker(results[0])
              matched = true
            } else if (results.length > 1) {
              hydrackerResults = results
              matched = true
            }
          }
          if (!matched) hydrackerNotFound = true
        }
      } catch(e) {
        console.error(e)
        hydrackerNotFound = true
      }
    }
  }

  async function confirmHydrackerID() {
    const id = parseInt(hydrackerManualId)
    if (!id) return
    hydrackerManualLoading = true
    try {
      const found = await HydrackerGetByID(id)
      if (found) {
        hydrackerNotFound = false
        await selectHydracker(found)
      }
    } catch(e) { console.error(e) }
    hydrackerManualLoading = false
  }

  function generateNFO() {
    if (!mediaInfo && !fileInfo && !selectedTMDB) return ''
    const W = 62
    const lines = []
    const pad = (k, v) => lines.push(`  ${k.padEnd(14)}: ${v}`)
    const section = (title) => {
      lines.push('')
      lines.push(`─── ${title} ${'─'.repeat(Math.max(0, W - 5 - title.length))}`)
    }
    const box = (text) => {
      const content = text.slice(0, W - 4)
      const inner = content.padEnd(W - 4)
      lines.push('╔' + '═'.repeat(W - 2) + '╗')
      lines.push(`║ ${inner} ║`)
      lines.push('╚' + '═'.repeat(W - 2) + '╝')
    }

    const title = selectedTMDB?.title || selectedTMDB?.name || selectedHydracker?.name || fileInfo?.title || ''
    const year = (selectedTMDB?.release_date || selectedTMDB?.first_air_date || '').slice(0, 4) || fileInfo?.year || ''
    box(year ? `${title} (${year})` : title || (file?.name || ''))

    section('GÉNÉRAL')
    if (file?.name)           pad('Fichier',    file.name)
    if (mediaInfo?.filesize)  pad('Taille',     mediaInfo.filesize)
    if (mediaInfo?.duration)  pad('Durée',      mediaInfo.duration)
    const qualName = qualityOptions.find(q => q.id === postQuality)?.name
    if (qualName)             pad('Qualité',    qualName)

    if (mediaInfo?.videoCodec || mediaInfo?.width) {
      section('VIDÉO')
      if (mediaInfo?.videoCodec) {
        const codec = mediaInfo.videoCodec + (mediaInfo.videoProfile ? ` (${mediaInfo.videoProfile})` : '')
        pad('Codec',      codec)
      }
      if (mediaInfo?.width && mediaInfo?.height) pad('Résolution', `${mediaInfo.width}×${mediaInfo.height}`)
      if (mediaInfo?.bitrate)   pad('Bitrate',    mediaInfo.bitrate)
    }

    if (mediaInfo?.audios?.length) {
      section('AUDIO')
      mediaInfo.audios.forEach((a, i) => {
        const val = [a.codec, a.channels, a.lang].filter(Boolean).join(' · ')
        pad(`Piste ${i + 1}`, val)
      })
      if (postLanguages.length) pad('Langues', postLanguages.map(l => l.name).join(', '))
    }

    if (mediaInfo?.subs?.length || postSubs.length) {
      section('SOUS-TITRES')
      if (postSubs.length) {
        postSubs.forEach(s => lines.push(`  • ${s.name}`))
      } else if (mediaInfo?.subs?.length) {
        mediaInfo.subs.forEach(s => lines.push(`  • ${s}`))
      }
    }

    if (selectedTMDB?.id || selectedHydracker?.id) {
      section('RÉFÉRENCES')
      if (selectedTMDB?.id) pad('TMDB',      `#${selectedTMDB.id}`)
      if (selectedHydracker?.id) pad('Hydracker', `#${selectedHydracker.id}`)
    }

    lines.push('')
    lines.push('─'.repeat(W))
    lines.push(`  Hydracker · ${new Date().toLocaleDateString('fr-FR')}`)
    return lines.join('\n')
  }
  $: nfoPreview = (file && (mediaInfo || fileInfo)) ? generateNFO() : ''

  // --- Post ---
  async function lancerPost() {
    if (!selectedHydracker || !postQuality || !postLanguages.length) return
    postLoading = true
    postResult = null
    nzbStatus = ''; nzbParparPct = 0; nzbNyuuPct = 0; nzbNyuuSpeed = ''; nzbNyuuETA = ''; nzbNyuuArticles = ''
    ddlHosts = { '1Fichier': { active: false, filename: '', pct: 0, speed: '', done: false, posting: false, posted: false, hydrackerID: 0, error: '' }, 'Send.now': { active: false, filename: '', pct: 0, speed: '', done: false, posting: false, posted: false, hydrackerID: 0, error: '' } }
    torrentState = { stage: '', msg: '', ftpPct: 0, ftpSpeed: 0, createPct: 0, seedboxPct: 0, seedboxSpeed: 0 }

    const titleID = selectedHydracker.id
    const langIDs = postLanguages.filter(l => l.id > 0).map(l => l.name)
    const subIDs  = postSubs.filter(s => s.id > 0).map(s => s.name)
    // Le site Hydracker rend le NFO en HTML — on wrappe dans <pre> pour préserver
    // les retours à la ligne et le formatage monospace.
    const nfoText = generateNFO()
    const nfo     = nfoText ? `<pre>${nfoText.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')}</pre>` : ''
    const errors = []
    const successes = []

    // Scroll vers les barres de progression dès le lancement
    requestAnimationFrame(() => {
      document.getElementById('progress-anchor')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })

    // Helper : relance une fois si l'appel échoue (sauf si l'utilisateur a cliqué Stop).
    // fn doit retourner une valeur truthy pour "OK" ; si throw OU falsy => retry.
    const withRetry = async (name, fn, successCheck) => {
      for (let attempt = 1; attempt <= 2; attempt++) {
        if (queueCancelled) throw new Error('annulé')
        try {
          const r = await fn()
          if (successCheck(r)) return r
          if (attempt === 2) throw new Error(`${name} : échec après retry`)
          addLog('QUEUE', `↻ Retry ${name} (tentative ${attempt + 1}/2)`)
        } catch(e) {
          if (queueCancelled) throw e
          if (attempt === 2) throw e
          addLog('QUEUE', `↻ Retry ${name} après erreur (${e}) — tentative ${attempt + 1}/2`)
        }
      }
    }

    // Étape 1 : Torrent d'abord (séquentiel)
    // - Mode "existing" (depuis Reseed) : upload direct du .torrent à Hydracker (pas de FTP/seedbox)
    // - Mode normal : ftpup + create + hydracker + seedbox
    if (postUploadTypes.torrent) {
      if (existingTorrentPath) {
        try {
          const r = await withRetry(
            'Torrent',
            () => PostExistingTorrent(titleID, postQuality, langIDs, subIDs, existingTorrentPath, nfo, postSeason, postEpisode),
            r => !!r?.hydracker_id,
          )
          successes.push(`Torrent #${r.hydracker_id} ajouté sur Hydracker (mode existant)`)
        } catch(e) { errors.push(`Torrent : ${e}`) }
      } else if (!mkvFilePath) errors.push('Torrent : chemin MKV introuvable')
      else {
        try {
          const r = await withRetry(
            'Torrent',
            () => PostTorrentWorkflow(titleID, postQuality, langIDs, subIDs, mkvFilePath, nfo, postSeason, postEpisode),
            r => !!r?.hydracker_id,
          )
          successes.push(`Torrent #${r.hydracker_id} ajouté + seedbox OK`)
        } catch(e) { errors.push(`Torrent : ${e}`) }
      }
    }

    const tasks = []
    if (postUploadTypes.nzb) {
      if (!mkvFilePath) errors.push('NZB : chemin du fichier introuvable — cliquez Parcourir')
      else tasks.push(
        withRetry(
          'NZB',
          () => PostNzbWorkflow(titleID, postQuality, langIDs, subIDs, mkvFilePath, nfo, postSeason, postEpisode),
          r => !!r?.nzb_path,
        )
          .then(r => successes.push(`NZB #${r.hydracker_id} ajouté`))
          .catch(e => errors.push(`NZB : ${e}`))
      )
    }
    if (postUploadTypes.ddl) {
      if (!mkvFilePath) errors.push('DDL : chemin MKV introuvable')
      else tasks.push(
        withRetry(
          'DDL',
          () => PostDDLWorkflow(titleID, postQuality, langIDs, subIDs, mkvFilePath, nfo, postDdlHosts.onefichier, postDdlHosts.sendcm, postSeason, postEpisode),
          r => !!(r?.links?.length),
        )
          .then(r => {
            const fname = file?.name || ''
            const ep = (postSeason || postEpisode) ? ` S${String(postSeason).padStart(2,'0')}E${String(postEpisode).padStart(2,'0')}` : ''
            const linksStr = r.links.join(' · ')
            successes.push(`DDL #${r.hydracker_id} ajouté (${r.links.length} lien${r.links.length > 1 ? 's' : ''}) — ${fname}${ep}\n${linksStr}`)
          })
          .catch(e => errors.push(`DDL : ${e}`))
      )
    }

    await Promise.all(tasks)

    if (successes.length && !errors.length) postResult = { ok: true, message: successes.join(' · ') }
    else if (successes.length) postResult = { ok: true, message: successes.join(' · '), details: errors.join(' | ') }
    else postResult = { ok: false, message: errors.join(' | ') }
    postLoading = false
    // Notif pour un post isolé (queue gère sa propre notif finale)
    if (!queueProcessing) {
      try { Notify(postResult.ok ? '✓ Post terminé' : '✗ Post échoué', file?.name || '') } catch(e) {}
    }
  }

  // Reset toutes les barres de progression (DDL / Torrent / NZB) à l'état initial
  function resetAllProgress() {
    nzbStatus = ''; nzbParparPct = 0; nzbNyuuPct = 0; nzbNyuuSpeed = ''; nzbNyuuETA = ''; nzbNyuuArticles = ''
    ddlHosts = {
      '1Fichier': { active: false, filename: '', pct: 0, speed: '', done: false, posting: false, posted: false, hydrackerID: 0, error: '' },
      'Send.now': { active: false, filename: '', pct: 0, speed: '', done: false, posting: false, posted: false, hydrackerID: 0, error: '' },
    }
    torrentState = { stage: '', msg: '', ftpPct: 0, ftpSpeed: 0, createPct: 0, seedboxPct: 0, seedboxSpeed: 0 }
  }

  async function stopPost() {
    queueCancelled = true
    queue = []
    try { await CancelAllWorkflows() } catch(e) {}
    postLoading = false
    queueProcessing = false
    queueCurrent = ''
    resetAllProgress()
    postResult = { ok: false, message: 'Arrêté par l\'utilisateur' }
    addLog('QUEUE', '■ Stop — tout arrêté, queue vidée')
    setTimeout(() => { queueCancelled = false }, 500)
  }

  // --- Hydracker ---
  async function searchHydracker() {
    if (!hydrackerSearchQuery.trim()) return
    hydrackerSearchLoading = true
    try {
      hydrackerResults = await HydrackerSearch(hydrackerSearchQuery) || []
      hydrackerSearchCache = [...hydrackerResults]
    } catch(e) { console.error(e) }
    hydrackerSearchLoading = false
  }

  async function selectHydracker(title) {
    selectedHydracker = title
    hydrackerResults = []
    hydrackerPosterUrl = ''
    if (title.poster) {
      try { hydrackerPosterUrl = await FetchImageBase64(title.poster) } catch(e) {}
    }
  }

  async function deselectHydracker() {
    selectedHydracker = null
    hydrackerPosterUrl = ''
    hydrackerNotFound = false
    if (hydrackerSearchCache.length) {
      hydrackerResults = [...hydrackerSearchCache]
    } else if (selectedTMDB) {
      const title = selectedTMDB.title || selectedTMDB.name
      if (title) {
        try {
          hydrackerSearchLoading = true
          hydrackerResults = await HydrackerSearch(title) || []
          hydrackerSearchCache = [...hydrackerResults]
        } catch(e) { console.error(e) }
        hydrackerSearchLoading = false
      }
      if (!hydrackerResults.length) hydrackerNotFound = true
    }
  }

  function addLang(opt) {
    if (!postLanguages.find(l => l.id === opt.id)) postLanguages = [...postLanguages, opt]
  }

  function addSub(opt) {
    if (!postSubs.find(s => s.id === opt.id)) postSubs = [...postSubs, opt]
  }
  function removeSubAt(idx) { postSubs = postSubs.filter((_, i) => i !== idx) }
  function removeLangAt(idx) { postLanguages = postLanguages.filter((_, i) => i !== idx) }
</script>

<div class="hydracker-tab">

  <!-- Queue batch -->
  {#if queueProcessing || queue.length > 0}
    <div class="queue-bar">
      <div class="queue-head">
        <span class="queue-title">⚡ Queue batch</span>
        <span class="queue-counter">{queueDone} fait{queueDone > 1 ? 's' : ''} · {queue.length + (queueProcessing ? 1 : 0)} en attente</span>
        <button class="btn-test" on:click={clearQueue} disabled={!queue.length}>Vider</button>
      </div>
      {#if queue.length > 0}
        <div class="queue-list">
          {#each queue as path, i}
            <div class="queue-item">
              <span class="queue-idx">#{i + 1}</span>
              <code class="queue-name">{path.split('/').pop()}</code>
              <button class="btn-x" on:click={() => dequeueAt(i)}>✕</button>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Zone de drop -->
  {#if !file}
  <div class="dropzone" class:drag={dragOver}
    on:dragover={onDragOver} on:dragleave={onDragLeave}
    role="region" aria-label="Zone de dépôt">
    <div class="drop-icon">🎬</div>
    <p>Glissez un fichier <strong>.mkv</strong> ici</p>
    <p class="drop-sub">ou</p>
    <button class="btn-browse" on:click={async () => {
      const paths = await SelectMkvFiles()
      if (!paths?.length) return
      if (paths.length === 1) loadFileFromPath(paths[0], null)
      else paths.forEach(p => enqueue(p))
    }}>Parcourir</button>
  </div>

  {:else}
  <div class="main-grid">

    <!-- Colonne gauche : poster + infos fiche -->
    <div class="col-left">
      <!-- Poster TMDB -->
      <div class="poster-wrap">
        {#if posterDataUrl}
          <img src={posterDataUrl} alt={selectedTMDB?.title || selectedTMDB?.name} />
        {:else}
          <div class="poster-placeholder">🎬</div>
        {/if}
      </div>
      {#if selectedTMDB}
        <div class="fiche-title">{selectedTMDB.title || selectedTMDB.name}</div>
        <div class="fiche-year">{(selectedTMDB.release_date || selectedTMDB.first_air_date || '').slice(0,4)}</div>
        <button class="btn-ghost-sm" on:click={() => { selectedTMDB = null; tmdbResults = [] }}>
          Changer de fiche
        </button>
      {/if}

      <!-- Confirmation fiche Hydracker -->
      {#if selectedHydracker}
        <div class="selected-hyd">
          {#if hydrackerPosterUrl}
            <img class="hyd-poster" src={hydrackerPosterUrl} alt={selectedHydracker.name} />
          {:else}
            <div class="hyd-poster hyd-poster-placeholder">🎬</div>
          {/if}
          <div class="hyd-info">
            <div class="hyd-info-name">✓ {selectedHydracker.name}</div>
            <div class="hyd-info-meta">
              {#if selectedHydracker.release_date}{selectedHydracker.release_date.slice(0,4)} · {/if}
              <span class="hyd-type badge-{selectedHydracker.type}">{selectedHydracker.type}</span>
            </div>
          </div>
          <button class="btn-x" style="margin-left:auto" on:click={deselectHydracker}>✕</button>
        </div>
      {:else if hydrackerNotFound && selectedTMDB}
        <div class="hyd-create-box">
          <div class="hyd-create-title">⚠ Fiche Hydracker introuvable</div>
          <div class="hyd-create-hint">Créez-la sur Hydracker Admin avec cet ID TMDB :</div>
          <div class="hyd-tmdb-url-row">
            <span class="hyd-tmdb-url">{selectedTMDB.id}</span>
            <button class="btn-copy" on:click={() => navigator.clipboard.writeText(String(selectedTMDB.id))}>📋</button>
          </div>
          <button class="btn-open-admin" on:click={() => OpenHydrackerAdmin()}>
            Ouvrir Hydracker Admin
          </button>
          <div class="hyd-create-hint" style="margin-top:10px">Après création, entrez l'ID de la fiche :</div>
          <div class="hyd-id-row">
            <input type="number" bind:value={hydrackerManualId} placeholder="ID fiche Hydracker"
              on:keydown={e => e.key === 'Enter' && confirmHydrackerID()} />
            <button class="btn-search" on:click={confirmHydrackerID} disabled={hydrackerManualLoading}>
              {hydrackerManualLoading ? '…' : 'Valider'}
            </button>
          </div>
        </div>
      {/if}

      <!-- Infos MediaInfo (repliable pour gagner de la place) -->
      {#if mediaInfoLoading}
        <div class="mi-loading">Analyse MediaInfo…</div>
      {:else if mediaInfoError}
        <div class="mi-error">⚠ MediaInfo : {mediaInfoError}</div>
      {:else if mediaInfo}
        <div class="mi-details" class:open={mediaInfoOpen}>
          <button type="button" class="mi-summary" on:click={() => mediaInfoOpen = !mediaInfoOpen}>
            <span class="mi-chevron">{mediaInfoOpen ? '▾' : '▸'}</span>
            🎞️ {mediaInfo.videoCodec || ''}{mediaInfo.width ? ` · ${mediaInfo.width}p` : ''}{mediaInfo.filesize ? ` · ${mediaInfo.filesize}` : ''}{mediaInfo.duration ? ` · ${mediaInfo.duration}` : ''}
          </button>
          {#if mediaInfoOpen}
            <div class="mi-block">
              {#if mediaInfo.filesize}<div class="mi-row"><span>Taille</span><span>{mediaInfo.filesize}</span></div>{/if}
              {#if mediaInfo.duration}<div class="mi-row"><span>Durée</span><span>{mediaInfo.duration}</span></div>{/if}
              {#if mediaInfo.videoCodec}<div class="mi-row"><span>Vidéo</span><span>{mediaInfo.videoCodec}{mediaInfo.videoProfile ? ' ' + mediaInfo.videoProfile : ''}</span></div>{/if}
              {#if mediaInfo.width && mediaInfo.height}<div class="mi-row"><span>Résolution</span><span>{mediaInfo.width}×{mediaInfo.height}</span></div>{/if}
              {#if mediaInfo.bitrate}<div class="mi-row"><span>Bitrate</span><span>{mediaInfo.bitrate}</span></div>{/if}
              {#each mediaInfo.audios as a}
                <div class="mi-row"><span>Audio{a.lang ? ' ('+a.lang+')' : ''}</span><span>{a.codec}{a.channels ? ' '+a.channels : ''}</span></div>
              {/each}
              {#if mediaInfo.subs.length}
                <div class="mi-row"><span>Sous-titres</span><span>{mediaInfo.subs.join(', ')}</span></div>
              {/if}
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Colonne droite : recherches + post options -->
    <div class="col-right">
      {#if queueCurrent || postLoading || queueProcessing}
        <div class="release-current">
          <span class="release-current-label">▶ {queueProcessing ? 'Release en cours' : 'Post en cours'}</span>
          <code class="release-current-name">{(queueCurrent || file?.name || '').split('/').pop()}</code>
          {#if queueProcessing && queue.length > 0}
            <button class="btn-skip-ep" on:click={async () => { try { await SkipCurrentEpisode() } catch(e) {} }} title="Abandonner cet épisode et passer au suivant dans la queue">
              ⏭ Skip épisode
            </button>
          {/if}
          <button class="btn-stop-inline" on:click={stopPost} title="Tout arrêter">■ Stop</button>
        </div>
      {/if}

      <!-- Fichier détecté -->
      <div class="file-badge">
        {existingTorrentPath ? '🧲' : '📄'} {file.name}
        {#if existingTorrentPath}
          <span style="color:#ffe066;font-size:10px;margin-left:6px">mode .torrent existant (pas de FTP/seedbox)</span>
        {/if}
        <button class="btn-x" on:click={() => { file = null; fileInfo = null; selectedTMDB = null; existingTorrentPath = '' }}>✕</button>
      </div>

      <!-- Actions principales (Lancer / Stop / Réinitialiser) juste sous le fichier -->
      <div class="post-actions">
        <button class="btn-start" title="⌘↵"
          disabled={postLoading || queueProcessing || (queue.length === 0 && (!postQuality || !postLanguages.length || !selectedHydracker || (!postUploadTypes.torrent && !postUploadTypes.nzb && !postUploadTypes.ddl)))}
          on:click={() => queue.length > 0 ? processQueue() : lancerPost()}>
          {postLoading || queueProcessing ? '…' : (queue.length > 0 ? `▶ Lancer la queue (${queue.length})` : '▶ Lancer')}
        </button>
        {#if postLoading || queueProcessing}
          <button class="btn-cancel" on:click={stopPost}>■ Stop</button>
        {/if}
        <button class="btn-reset" on:click={async () => {
          await stopPost()
          await new Promise(r => setTimeout(r, 300))
          queueResults = []; queueDone = 0; queueTotal = 0
          file = null; fileInfo = null; selectedTMDB = null;
          selectedHydracker = null; mediaInfo = null; posterDataUrl = ''; hydrackerPosterUrl = '';
          postQuality = 0; postLanguages = []; postSubs = [];
          postSeason = 0; postEpisode = 0;
          mkvFilePath = ''; existingTorrentPath = ''; postResult = null;
        }}>↺ Réinitialiser</button>
      </div>

      <!-- Résumé du parser -->
      {#if fileInfo}
        <div class="parser-row">
          {#if fileInfo.title}<span class="tag tag-title">{fileInfo.title}</span>{/if}
          {#if fileInfo.year}<span class="tag">{fileInfo.year}</span>{/if}
          {#if fileInfo.quality}<span class="tag tag-qual">{fileInfo.quality}</span>{/if}
          {#if fileInfo.source}<span class="tag">{fileInfo.source}</span>{/if}
          {#if fileInfo.video_codec}<span class="tag tag-codec">{fileInfo.video_codec}</span>{/if}
          {#if fileInfo.audio_codec}<span class="tag tag-codec">{fileInfo.audio_codec}</span>{/if}
          {#each (fileInfo.languages || []) as l}<span class="tag tag-lang">{l}</span>{/each}
        </div>
      {/if}

      <!-- Ambiguïté TMDB -->
      {#if tmdbAmbiguous && tmdbResults.length > 1}
        <div class="ambig-box">
          <div class="ambig-title">⚠️ Plusieurs résultats — choisissez la bonne fiche :</div>
          <div class="ambig-list">
            {#each tmdbResults.slice(0,8) as m}
              <button class="ambig-item" on:click={() => selectTMDB(m)}>
                {#if m.poster_path}
                  <img src="https://image.tmdb.org/t/p/w92{m.poster_path}" alt="" />
                {:else if m._poster_full}
                  <img src={m._poster_full} alt="" />
                {:else}
                  <div class="ambig-no-poster">🎬</div>
                {/if}
                <div>
                  <div class="ambig-name">{m.title || m.name}</div>
                  <div class="ambig-year">{(m.release_date || m.first_air_date || '').slice(0,4)} · {m.media_type}</div>
                </div>
              </button>
            {/each}
          </div>
        </div>
      {/if}

      <!-- Recherches TMDB + Hydracker côte à côte -->
      <div class="searches-row">
        <div class="search-section">
          <div class="search-label">🔍 Recherche TMDB</div>
          <div class="search-row">
            <input type="text" bind:value={tmdbSearchQuery} placeholder="Nom du film/série"
              on:keydown={e => e.key === 'Enter' && manualTMDBSearch()} />
            <input type="text" bind:value={tmdbSearchId} placeholder="ID TMDB" style="width:90px;flex:none"
              on:keydown={e => e.key === 'Enter' && manualTMDBSearch()} />
            <button class="btn-search" on:click={manualTMDBSearch} disabled={tmdbSearchLoading}>
              {tmdbSearchLoading ? '…' : 'Chercher'}
            </button>
          </div>
        </div>

        <div class="search-section">
          <div class="search-label">🌊 Recherche Hydracker</div>
          <div class="search-row">
            <input type="text" class="hyd-search-input" bind:value={hydrackerSearchQuery} placeholder="Nom sur Hydracker (⌘K)"
              on:keydown={e => e.key === 'Enter' && searchHydracker()} />
            <button class="btn-search" on:click={searchHydracker} disabled={hydrackerSearchLoading}>
              {hydrackerSearchLoading ? '…' : 'Chercher'}
            </button>
          </div>
          {#if hydrackerResults.length > 0}
            <div class="hydracker-results">
              {#each hydrackerResults as t}
                <button class="hydracker-item" on:click={() => selectHydracker(t)}>
                  <span class="hyd-name">{t.name}</span>
                  <span class="hyd-year">{(t.release_date||'').slice(0,4)}</span>
                  <span class="hyd-type badge-{t.type}">{t.type}</span>
                </button>
              {/each}
              <button class="hydracker-item hyd-create-btn" on:click={() => { hydrackerResults = []; hydrackerNotFound = true }}>
                <span class="hyd-name">+ Aucune de ces fiches — en créer une</span>
              </button>
            </div>
          {/if}
        </div>
      </div>

      <!-- Options de post -->
      {#if selectedTMDB}
        <div class="post-options">
          <div class="post-label">Options de post</div>

          <div class="post-grid">
            <!-- Colonne gauche -->
            <div class="post-field">
              <label for="post-quality">Qualité</label>
              <select id="post-quality" bind:value={postQuality}>
                <option value={0}>-- Choisir --</option>
                {#each qualityOptions as q}
                  <option value={q.id}>{q.name}</option>
                {/each}
              </select>
            </div>

            <!-- Colonne droite -->
            {#if selectedHydracker?.type === 'tv' || postSeason > 0 || postEpisode > 0}
              <div class="post-field">
                <div class="post-field-label">Saison / Épisode</div>
                <div class="se-row">
                  <label class="se-label">Saison <input type="number" min="0" class="se-input" bind:value={postSeason} /></label>
                  <label class="se-label">Épisode <input type="number" min="0" class="se-input" bind:value={postEpisode} /></label>
                </div>
              </div>
            {:else}<div></div>{/if}

            <div class="post-field">
              <div class="post-field-label">Langues</div>
              <div class="chips-row">
                {#each postLanguages as l, i}
                  <span class="chip" class:chip-unknown={l.id === 0} title={l.id === 0 ? 'Langue non reconnue — ne sera pas envoyée' : ''}>{l.name}{#if l.id === 0} ⚠{/if}<button class="chip-x" on:click={() => removeLangAt(i)}>✕</button></span>
                {/each}
                {#if !postLanguages.length}<span class="chips-empty">Aucune détectée</span>{/if}
              </div>
              <div class="add-row">
                <select bind:value={langSelectValue}>
                  <option value={null}>— Ajouter une langue —</option>
                  {#each langOptions.filter(o => !postLanguages.find(l => l.id === o.id)) as o}
                    <option value={o}>{o.name}</option>
                  {/each}
                </select>
                <button class="btn-add" on:click={() => { if (langSelectValue) { addLang(langSelectValue); langSelectValue = null } }}>+</button>
              </div>
            </div>

            <div class="post-field">
              <div class="post-field-label">Sous-titres</div>
              <div class="chips-row">
                {#each postSubs as s, i}
                  <span class="chip" class:chip-unknown={s.id === 0}>{s.name}{#if s.id === 0} ⚠{/if}<button class="chip-x" on:click={() => removeSubAt(i)}>✕</button></span>
                {/each}
                {#if !postSubs.length}<span class="chips-empty">Aucun détecté</span>{/if}
              </div>
              <div class="add-row">
                <select bind:value={subSelectValue}>
                  <option value={null}>— Ajouter un sous-titre —</option>
                  {#each subOptions.filter(o => !postSubs.find(s => s.id === o.id)) as o}
                    <option value={o}>{o.name}</option>
                  {/each}
                </select>
                <button class="btn-add" on:click={() => { if (subSelectValue) { addSub(subSelectValue); subSelectValue = null } }}>+</button>
              </div>
            </div>

            <div class="post-field">
              <div class="post-field-label">Uploader via</div>
              <div class="upload-types">
                <label class="check-label"><input type="checkbox" bind:checked={postUploadTypes.torrent} /> Torrent</label>
                <label class="check-label"><input type="checkbox" bind:checked={postUploadTypes.nzb} /> NZB</label>
                <label class="check-label"><input type="checkbox" bind:checked={postUploadTypes.ddl} /> DDL</label>
                <button class="btn-full-auto" class:active={postUploadTypes.torrent && postUploadTypes.nzb && postUploadTypes.ddl}
                  on:click={() => { const on = !(postUploadTypes.torrent && postUploadTypes.nzb && postUploadTypes.ddl); postUploadTypes = { torrent: on, nzb: on, ddl: on } }}>
                  ⚡ Full Auto
                </button>
              </div>
            </div>

            {#if postUploadTypes.ddl}
              <div class="post-field">
                <div class="post-field-label">Hosts DDL</div>
                <div class="upload-types">
                  <label class="check-label"><input type="checkbox" bind:checked={postDdlHosts.onefichier} /> 1Fichier</label>
                  <label class="check-label"><input type="checkbox" bind:checked={postDdlHosts.sendcm} /> Send.now</label>
                </div>
              </div>
            {:else}<div></div>{/if}
          </div><!-- /.post-grid -->

          <!-- Ancre pour auto-scroll vers les barres de progression -->
          <div id="progress-anchor"></div>

          <!-- Barres DDL juste sous les checkboxes -->
          {#if postUploadTypes.ddl && Object.values(ddlHosts).some(h => h.active || h.done || h.error || h.posting)}
            <div class="ddl-bars">
              {#each Object.entries(ddlHosts) as [host, h]}
                {#if h.active || h.done || h.error || h.posting || h.posted}
                  <div class="ddl-bar-card" class:done={!!h.posted} class:error={!!h.error}>
                    <div class="ddl-bar-header">
                      <span class="ddl-bar-host">{host}</span>
                      {#if h.skipped}
                        <span class="ddl-bar-status skipped">⏭ Skippé</span>
                      {:else if h.error}
                        <span class="ddl-bar-status err">✗ Erreur</span>
                      {:else if h.posted}
                        <span class="ddl-bar-status ok">✓ Posté sur Hydracker{#if h.hydrackerID} #{h.hydrackerID}{/if}</span>
                      {:else if h.posting}
                        <span class="ddl-bar-status posting">⬆ Post Hydracker…</span>
                      {:else if h.done}
                        <span class="ddl-bar-status ok">✓ Upload terminé</span>
                      {:else}
                        <span class="ddl-bar-speed">{h.speed}</span>
                      {/if}
                      {#if h.active && !h.done && !h.posted && !h.error}
                        <button class="btn-skip-host" title="Skipper cet hébergeur (trop lent) — passe à l'épisode suivant après l'autre hébergeur"
                          on:click={async () => { try { await CancelDDLHost(host) } catch(e) {} }}>
                          ✕ Skip
                        </button>
                      {/if}
                    </div>
                    <div class="ddl-bar-filename">{h.filename}</div>
                    <div class="ddl-bar-row">
                      <div class="progress-bar" style="flex:1">
                        <div class="progress-fill"
                             class:posting={h.posting && !h.posted}
                             class:hydone={!!h.posted}
                             style="width:{h.posted || h.posting || h.done ? 100 : h.pct}%"></div>
                      </div>
                      <span class="ddl-bar-pct">{h.posted || h.posting || h.done ? '100' : h.pct.toFixed(0)}%</span>
                    </div>
                    {#if h.error}<div class="ddl-bar-errmsg">{h.error}</div>{/if}
                  </div>
                {/if}
              {/each}
            </div>
          {/if}

          <!-- Barres Torrent -->
          {#if postUploadTypes.torrent && (torrentState.stage || torrentState.ftpPct > 0)}
            <div class="ddl-bars">
              <div class="ddl-bar-card">
                <div class="ddl-bar-header">
                  <span class="ddl-bar-host">Torrent</span>
                  <span class="ddl-bar-speed">{torrentState.msg}</span>
                </div>

                <div class="ddl-step">
                  <div class="ddl-step-label">
                    <span>1. Upload FTP</span>
                    <span class="ddl-bar-speed">{torrentState.ftpSpeed.toFixed(1)} MB/s · {torrentState.ftpPct.toFixed(0)}%</span>
                  </div>
                  <div class="progress-bar"><div class="progress-fill" style="width:{torrentState.ftpPct}%"></div></div>
                </div>

                {#if torrentState.createPct > 0 || ['create','post','download','seedbox','done'].includes(torrentState.stage)}
                  <div class="ddl-step">
                    <div class="ddl-step-label">
                      <span>2. Création .torrent</span>
                      <span class="ddl-bar-speed">{torrentState.createPct.toFixed(0)}%</span>
                    </div>
                    <div class="progress-bar"><div class="progress-fill" style="width:{torrentState.createPct}%"></div></div>
                  </div>
                {/if}

                {#if ['post','post_done','download','download_done','seedbox','done'].includes(torrentState.stage)}
                  <div class="ddl-step">
                    <div class="ddl-step-label">
                      <span>3. Post Hydracker</span>
                      <span class="ddl-bar-status {['post_done','download','download_done','seedbox','done'].includes(torrentState.stage) ? 'ok' : 'posting'}">
                        {['post_done','download','download_done','seedbox','done'].includes(torrentState.stage) ? '✓ Posté' : '⬆ Envoi…'}
                      </span>
                    </div>
                  </div>
                {/if}

                {#if ['seedbox','done'].includes(torrentState.stage)}
                  <div class="ddl-step">
                    <div class="ddl-step-label">
                      <span>4. Upload seedbox</span>
                      <span class="ddl-bar-speed">{torrentState.seedboxSpeed.toFixed(1)} MB/s · {torrentState.seedboxPct.toFixed(0)}%</span>
                    </div>
                    <div class="progress-bar"><div class="progress-fill" class:hydone={torrentState.stage === 'done'} style="width:{torrentState.seedboxPct}%"></div></div>
                  </div>
                {/if}
              </div>
            </div>
          {/if}

          <!-- Barres NZB (à l'intérieur des post-options) -->
          {#if postUploadTypes.nzb && nzbStatus}
            <div class="ddl-bars">
              <div class="ddl-bar-card" class:done={nzbStatus === 'Terminé'}>
                <div class="ddl-bar-header">
                  <span class="ddl-bar-host">NZB</span>
                  <span class="ddl-bar-speed">{nzbStatus}</span>
                </div>
                {#if nzbParparPct > 0 && nzbParparPct < 100}
                  <div class="ddl-step">
                    <div class="ddl-step-label">
                      <span>1. PAR2</span>
                      <span class="ddl-bar-speed">{nzbParparPct.toFixed(0)}%</span>
                    </div>
                    <div class="progress-bar"><div class="progress-fill" style="width:{nzbParparPct}%"></div></div>
                  </div>
                {/if}
                {#if nzbNyuuPct > 0}
                  <div class="ddl-step">
                    <div class="ddl-step-label">
                      <span>2. Usenet</span>
                      <span class="ddl-bar-speed">
                        {#if nzbNyuuArticles}{nzbNyuuArticles} · {/if}
                        {#if nzbNyuuSpeed}{nzbNyuuSpeed} · {/if}
                        {#if nzbNyuuETA}ETA {nzbNyuuETA} · {/if}
                        {nzbNyuuPct.toFixed(0)}%
                      </span>
                    </div>
                    <div class="progress-bar"><div class="progress-fill" class:hydone={nzbNyuuPct >= 100} style="width:{nzbNyuuPct}%"></div></div>
                  </div>
                {/if}
              </div>
            </div>
          {/if}

        </div>

        <!-- Résultat -->
        {#if postResult}
          <div class="post-result" class:post-result-ok={postResult.ok} class:post-result-err={!postResult.ok}>
            {postResult.ok ? '✓' : '✗'} {postResult.message}
            {#if postResult.details}<div class="post-result-details">{postResult.details}</div>{/if}
          </div>
        {/if}

        <!-- Récapitulatif du post (repliable) -->
        <div class="recap-box" class:open={recapOpen}>
          <button type="button" class="recap-title" on:click={() => recapOpen = !recapOpen}>
            <span class="mi-chevron">{recapOpen ? '▾' : '▸'}</span>
            📋 Récapitulatif du post Hydracker
          </button>
          {#if recapOpen}
            <div class="recap-body">
              <div class="recap-row">
                <span class="recap-key">Titre</span>
                <span class="recap-val">{selectedTMDB.title || selectedTMDB.name}
                  {#if (selectedTMDB.release_date || selectedTMDB.first_air_date)}
                    ({(selectedTMDB.release_date || selectedTMDB.first_air_date).slice(0,4)})
                  {/if}
                </span>
              </div>
              <div class="recap-row">
                <span class="recap-key">TMDB ID</span>
                <span class="recap-val recap-id">{selectedTMDB.id} <span class="recap-type">{selectedTMDB.media_type || 'movie'}</span></span>
              </div>
              <div class="recap-row">
                <span class="recap-key">Fiche Hydracker</span>
                <span class="recap-val">{selectedHydracker ? selectedHydracker.name + ' (#' + (selectedHydracker.id || '?') + ')' : '— non sélectionnée'}</span>
              </div>
              <div class="recap-row">
                <span class="recap-key">Qualité</span>
                <span class="recap-val">{qualityOptions.find(q => q.id === postQuality)?.name || '—'}</span>
              </div>
              <div class="recap-row">
                <span class="recap-key">Codec vidéo</span>
                <span class="recap-val">{mediaInfo?.videoCodec || fileInfo?.video_codec || '—'}</span>
              </div>
              <div class="recap-row">
                <span class="recap-key">Codec audio</span>
                <span class="recap-val">{mediaInfo?.audios?.[0]?.codec || fileInfo?.audio_codec || '—'}</span>
              </div>
              <div class="recap-row">
                <span class="recap-key">Langues</span>
                <span class="recap-val">{postLanguages.length ? postLanguages.map(l => l.name).join(', ') : '—'}</span>
              </div>
              <div class="recap-row">
                <span class="recap-key">Sous-titres</span>
                <span class="recap-val">{postSubs.length ? postSubs.map(s => s.name).join(', ') : '—'}</span>
              </div>
              <div class="recap-row">
                <span class="recap-key">Upload via</span>
                <span class="recap-val">
                  {(() => {
                    const parts = []
                    if (postUploadTypes.torrent) parts.push('Torrent')
                    if (postUploadTypes.nzb) parts.push('NZB')
                    if (postUploadTypes.ddl) {
                      const hosts = []
                      if (postDdlHosts.onefichier) hosts.push('1Fichier')
                      if (postDdlHosts.sendcm) hosts.push('Send.now')
                      parts.push(hosts.length ? `DDL (${hosts.join(' + ')})` : 'DDL')
                    }
                    return parts.join(' + ') || '—'
                  })()}
                </span>
              </div>
              <div class="recap-row">
                <span class="recap-key">Fichier</span>
                <span class="recap-val recap-file">{file?.name}</span>
              </div>
            </div>
          {/if}
        </div>

        {#if nfoPreview}
          <div class="nfo-preview" class:open={nfoOpen}>
            <button type="button" class="nfo-preview-header" on:click={() => nfoOpen = !nfoOpen}>
              <span class="mi-chevron">{nfoOpen ? '▾' : '▸'}</span>
              <span>📝 NFO (inclus dans NZB + DDL + Torrent)</span>
              <button class="btn-copy" title="Copier" on:click|stopPropagation={() => navigator.clipboard.writeText(nfoPreview)}>📋</button>
            </button>
            {#if nfoOpen}
              <pre class="nfo-preview-body">{nfoPreview}</pre>
            {/if}
          </div>
        {/if}
      {/if}

    </div>
  </div>
  {/if}
</div>

<style>
  .hydracker-tab { height: 100%; overflow-y: auto; padding: 24px; }

  .queue-bar {
    margin-bottom: 18px;
    background: linear-gradient(180deg, rgba(255, 214, 10, 0.06) 0%, rgba(255, 214, 10, 0.02) 100%);
    border: 1px solid rgba(255, 214, 10, 0.25);
    border-radius: 12px;
    padding: 12px 14px;
  }
  .queue-head { display: flex; align-items: center; gap: 12px; }
  .queue-title { font-size: 12px; font-weight: 700; color: var(--yellow); text-transform: uppercase; letter-spacing: 1.2px; }
  .queue-counter { font-size: 11px; color: var(--text2); flex: 1; }
  .release-current {
    display: flex; align-items: center; gap: 10px;
    padding: 8px 14px; margin-bottom: 8px;
    background: linear-gradient(90deg, rgba(255,214,10,0.08), transparent);
    border: 1px solid rgba(255,214,10,0.3);
    border-radius: 8px;
  }
  .release-current-label {
    font-size: 11px; font-weight: 700; color: #ffe066;
    text-transform: uppercase; letter-spacing: 1px; flex: none;
  }
  .release-current-name {
    font-family: ui-monospace, Menlo, monospace; font-size: 11px;
    color: #7ef0c0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    min-width: 0; flex: 1;
  }
  .btn-stop-inline {
    background: #dc2626; color: white; border: 0;
    padding: 5px 12px; font-size: 11px; font-weight: 700;
    letter-spacing: 0.5px; border-radius: 6px; cursor: pointer; flex: none;
  }
  .btn-stop-inline:hover { background: #ef4444; }
  .btn-skip-ep {
    background: rgba(255,214,10,0.15); color: #ffe066;
    border: 1px solid rgba(255,214,10,0.4);
    padding: 5px 10px; font-size: 11px; font-weight: 600;
    border-radius: 6px; cursor: pointer; flex: none;
  }
  .btn-skip-ep:hover { background: rgba(255,214,10,0.25); }
  .btn-skip-host {
    background: transparent; color: var(--text3);
    border: 1px solid rgba(255,255,255,0.1);
    padding: 2px 7px; font-size: 10px; font-weight: 600;
    border-radius: 5px; cursor: pointer; flex: none;
  }
  .btn-skip-host:hover { background: rgba(239,68,68,0.15); color: #ef4444; border-color: rgba(239,68,68,0.3); }
  .queue-list {
    display: grid; grid-template-columns: 1fr 1fr; gap: 4px 8px; margin-top: 8px;
    max-height: 100px;   /* ~3 lignes × 2 cols = 6 items visibles */
    overflow-y: auto;
    padding-right: 4px;
  }
  .queue-list::-webkit-scrollbar { width: 6px; }
  .queue-list::-webkit-scrollbar-track { background: rgba(255,255,255,0.02); border-radius: 3px; }
  .queue-list::-webkit-scrollbar-thumb { background: rgba(255,255,255,0.12); border-radius: 3px; }
  .queue-list::-webkit-scrollbar-thumb:hover { background: rgba(255,255,255,0.2); }
  .queue-item { display: flex; align-items: center; gap: 8px; padding: 4px 8px; background: rgba(0,0,0,0.2); border-radius: 6px; min-width: 0; }
  .queue-item .queue-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; flex: 1; }
  @media (max-width: 900px) { .queue-list { grid-template-columns: 1fr; max-height: 200px; } }
  .queue-idx { font-size: 10px; color: var(--text3); width: 30px; flex: none; font-variant-numeric: tabular-nums; }
  .queue-name { flex: 1; font-family: ui-monospace, Menlo, monospace; font-size: 11px; color: var(--text2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  /* Shared card surface */
  .card-surface {
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px solid var(--border);
    border-radius: 14px;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.05), 0 1px 2px rgba(0,0,0,0.4);
  }

  /* Drop zone */
  .dropzone {
    border: 2px dashed var(--border-strong); border-radius: 16px;
    padding: 60px 40px; text-align: center; cursor: pointer;
    transition: all 200ms ease; color: var(--text3);
    background: linear-gradient(180deg, rgba(255,255,255,0.02) 0%, rgba(255,255,255,0.005) 100%);
  }
  .dropzone.drag {
    border-color: rgba(0, 180, 216, 0.55);
    background: rgba(0, 180, 216, 0.06);
    color: var(--blue-hot);
  }
  .drop-icon { font-size: 48px; margin-bottom: 12px; }
  .dropzone p { margin-bottom: 6px; font-size: 15px; color: var(--text2); }
  .drop-sub { font-size: 12px; color: var(--text3); }
  .btn-browse {
    display: inline-block; margin-top: 14px;
    color: #fff; background: var(--grad-primary);
    border: 1px solid rgba(0,0,0,0.25);
    padding: 10px 22px; border-radius: 10px;
    font-size: 13px; font-weight: 600; letter-spacing: 0.2px;
    box-shadow:
      inset 0 1px 0 rgba(255,255,255,0.25),
      inset 0 -1px 0 rgba(0,0,0,0.2),
      0 1px 2px rgba(0,0,0,0.4),
      0 8px 24px -6px var(--accent-glow);
  }
  .btn-browse:hover { background: var(--grad-primary-hover); filter: brightness(1.05); }
  .btn-browse:active { transform: translateY(1px); }

  /* Main grid — layout 2 colonnes pour éviter de scroller */
  .main-grid { display: grid; grid-template-columns: 260px 1fr; gap: 18px; align-items: start; }
  @media (max-width: 1100px) {
    .main-grid { grid-template-columns: 1fr; }
  }

  /* Left col */
  .col-left { display: flex; flex-direction: column; gap: 10px; }
  .poster-wrap {
    width: 260px; aspect-ratio: 2/3; border-radius: 10px; overflow: hidden;
    background: var(--bg2);
    border: 1px solid var(--border);
    box-shadow: 0 6px 24px -8px rgba(0,0,0,0.6);
  }
  .poster-wrap img { width: 100%; height: 100%; object-fit: cover; }
  .poster-placeholder {
    width: 100%; height: 100%;
    display: flex; align-items: center; justify-content: center;
    font-size: 60px; color: var(--text3);
  }
  .fiche-title { font-weight: 700; font-size: 14px; color: var(--text); letter-spacing: -0.01em; }
  .fiche-year { font-size: 12px; color: var(--text3); }
  .btn-ghost-sm {
    background: rgba(255,255,255,0.04);
    border: 1px solid var(--border);
    color: var(--text2);
    padding: 5px 11px; font-size: 11px; border-radius: 7px;
    text-transform: uppercase; letter-spacing: 0.4px;
    transition: all 160ms ease;
  }
  .btn-ghost-sm:hover {
    color: var(--text);
    background: rgba(0, 180, 216, 0.08);
    border-color: rgba(0, 180, 216, 0.35);
  }

  .mi-loading { font-size: 12px; color: var(--text3); }
  .mi-error {
    font-size: 11px; color: #ff9585;
    background: rgba(239, 68, 68, 0.08);
    border: 1px solid rgba(239, 68, 68, 0.25);
    border-radius: 8px; padding: 9px 11px;
  }
  .mi-details { background: var(--bg2); border: 1px solid var(--border); border-radius: 8px; overflow: hidden; }
  .mi-summary {
    cursor: pointer; padding: 8px 12px; font-size: 11px; color: var(--text2);
    background: transparent; border: 0; width: 100%; text-align: left;
    display: flex; align-items: center; gap: 6px; user-select: none;
  }
  .mi-summary:hover { background: rgba(255,255,255,0.03); }
  .mi-chevron { color: var(--text3); font-size: 10px; display: inline-block; width: 10px; }
  .mi-details.open .mi-summary { border-bottom: 1px solid var(--border); }
  .mi-details .mi-block { padding: 8px 12px; }

  .mi-block {
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px solid var(--border);
    border-radius: 12px; padding: 13px; font-size: 11px;
  }
  .mi-title {
    font-weight: 600; color: var(--text2);
    margin-bottom: 10px; font-size: 10px;
    text-transform: uppercase; letter-spacing: 1.2px;
  }
  .mi-row {
    display: flex; justify-content: space-between;
    padding: 4px 0; border-bottom: 1px solid rgba(255,255,255,0.05);
    color: var(--text3);
  }
  .mi-row:last-child { border-bottom: none; }
  .mi-row span:last-child { color: var(--text); text-align: right; max-width: 120px; }

  /* Right col */
  .col-right { display: flex; flex-direction: column; gap: 14px; }

  .file-badge {
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 9px 13px; font-size: 12px; color: var(--text2);
    display: flex; align-items: center; justify-content: space-between; gap: 8px;
    word-break: break-all;
  }
  .btn-x {
    background: transparent; color: var(--text3); font-size: 14px;
    padding: 2px 7px; border-radius: 6px;
    transition: all 140ms ease;
  }
  .btn-x:hover { color: var(--red-hot); background: rgba(239, 68, 68, 0.08); }

  .parser-row { display: flex; flex-wrap: wrap; gap: 6px; }
  .tag {
    background: rgba(255,255,255,0.05);
    border: 1px solid var(--border);
    border-radius: 6px; padding: 3px 9px; font-size: 11px; color: var(--text2);
  }
  .tag-title { background: rgba(0, 180, 216, 0.12); color: var(--blue-hot); font-weight: 600; border-color: rgba(0, 180, 216, 0.25); }
  .tag-qual { background: rgba(230, 57, 70, 0.12); color: var(--red-hot); border-color: rgba(230, 57, 70, 0.25); }
  .tag-codec { background: rgba(34, 197, 94, 0.12); color: #7ef0c0; border-color: rgba(34, 197, 94, 0.25); }
  .tag-lang { background: rgba(247, 127, 0, 0.12); color: var(--orange); border-color: rgba(247, 127, 0, 0.25); }

  /* Ambiguïté */
  .ambig-box {
    background: rgba(255, 214, 10, 0.05);
    border: 1px solid rgba(255, 214, 10, 0.25);
    border-radius: 12px; padding: 14px;
  }
  .ambig-title { font-size: 12px; color: var(--yellow); margin-bottom: 10px; }
  .ambig-list { display: flex; flex-direction: column; gap: 6px; max-height: 220px; overflow-y: auto; }
  .ambig-item {
    display: flex; align-items: center; gap: 10px;
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    border-radius: 8px; padding: 7px 11px; text-align: left;
    transition: all 160ms ease;
  }
  .ambig-item:hover {
    background: rgba(255,255,255,0.06);
    border-color: rgba(0, 180, 216, 0.35);
  }
  .ambig-item img { width: 36px; height: 54px; object-fit: cover; border-radius: 4px; }
  .ambig-no-poster {
    width: 36px; height: 54px;
    background: var(--bg2); border-radius: 4px;
    display: flex; align-items: center; justify-content: center; font-size: 18px;
  }
  .ambig-name { font-size: 13px; font-weight: 500; color: var(--text); }
  .ambig-year { font-size: 11px; color: var(--text3); }

  /* Recherches */
  .search-section {
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px solid var(--border);
    border-radius: 12px; padding: 14px;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.05);
  }
  .search-label {
    font-size: 10px; font-weight: 600; color: var(--text2);
    margin-bottom: 10px; text-transform: uppercase; letter-spacing: 1.2px;
  }
  .search-row { display: flex; gap: 8px; }
  .search-row input { flex: 1; }
  .btn-search {
    color: #fff; background: var(--grad-primary);
    border: 1px solid rgba(0,0,0,0.25);
    padding: 8px 18px; flex: none; font-weight: 600;
    box-shadow:
      inset 0 1px 0 rgba(255,255,255,0.25),
      inset 0 -1px 0 rgba(0,0,0,0.2),
      0 1px 2px rgba(0,0,0,0.4),
      0 6px 18px -6px var(--accent-glow);
  }
  .btn-search:hover:not(:disabled) { background: var(--grad-primary-hover); filter: brightness(1.05); }
  .btn-search:disabled { opacity: 0.5; cursor: default; box-shadow: none; }

  .hydracker-results { margin-top: 10px; display: flex; flex-direction: column; gap: 4px; max-height: 200px; overflow-y: auto; }
  .hydracker-item {
    display: flex; align-items: center; gap: 10px;
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    border-radius: 8px; padding: 8px 11px; text-align: left;
    transition: all 160ms ease;
  }
  .hydracker-item:hover {
    background: rgba(255,255,255,0.06);
    border-color: rgba(0, 180, 216, 0.35);
  }
  .hyd-create-btn {
    background: rgba(255, 214, 10, 0.05);
    border: 1px dashed rgba(255, 214, 10, 0.4);
  }
  .hyd-create-btn:hover { background: rgba(255, 214, 10, 0.1); border-color: rgba(255, 214, 10, 0.6); }
  .hyd-create-btn .hyd-name { color: var(--yellow); }
  .hyd-name { flex: 1; font-size: 13px; color: var(--text); }
  .hyd-year { font-size: 11px; color: var(--text3); }
  .hyd-type { font-size: 10px; padding: 2px 7px; border-radius: 9999px; margin-left: 6px; font-weight: 500; }
  .badge-movie { background: rgba(0, 180, 216, 0.15); color: var(--blue-hot); }
  .badge-series { background: rgba(34, 197, 94, 0.15); color: #7ef0c0; }
  .selected-hyd {
    font-size: 12px; color: #7ef0c0; margin-top: 8px;
    display: flex; align-items: center; gap: 10px;
    background: rgba(34, 197, 94, 0.08);
    border: 1px solid rgba(34, 197, 94, 0.3);
    border-radius: 10px; padding: 9px 11px;
  }
  .hyd-poster { width: 40px; height: 60px; object-fit: cover; border-radius: 5px; flex: none; }
  .hyd-poster-placeholder {
    background: var(--bg2);
    display: flex; align-items: center; justify-content: center;
    font-size: 18px; color: var(--text3);
  }
  .hyd-info { display: flex; flex-direction: column; gap: 3px; }
  .hyd-info-name { font-weight: 600; color: #7ef0c0; font-size: 13px; }
  .hyd-info-meta { font-size: 11px; color: var(--text3); display: flex; align-items: center; gap: 4px; }
  .hyd-create-box {
    margin-top: 8px;
    background: rgba(255, 214, 10, 0.05);
    border: 1px solid rgba(255, 214, 10, 0.25);
    border-radius: 12px; padding: 14px;
    display: flex; flex-direction: column; gap: 8px;
  }
  .hyd-create-title { font-size: 13px; font-weight: 600; color: var(--yellow); }
  .hyd-create-hint { font-size: 11px; color: var(--text2); }
  .hyd-tmdb-url-row {
    display: flex; align-items: center; gap: 6px;
    background: rgba(0,0,0,0.25);
    border-radius: 6px; padding: 6px 9px;
  }
  .hyd-tmdb-url { font-size: 11px; color: var(--blue-hot); word-break: break-all; flex: 1; }
  .btn-copy {
    background: rgba(255,255,255,0.04);
    border: 1px solid var(--border);
    color: var(--text2); border-radius: 5px; padding: 3px 7px;
    cursor: pointer; font-size: 13px; flex: none;
  }
  .btn-copy:hover { background: rgba(255,255,255,0.08); color: var(--text); }
  .btn-open-admin {
    color: #fff; background: linear-gradient(180deg, #f77f00 0%, #d85e00 100%);
    border: 1px solid rgba(0,0,0,0.25);
    border-radius: 9px; padding: 8px 14px;
    cursor: pointer; font-size: 12px; font-weight: 600; text-align: center;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.2), 0 6px 18px -6px rgba(247, 127, 0, 0.4);
  }
  .btn-open-admin:hover { filter: brightness(1.08); }
  .hyd-id-row { display: flex; gap: 6px; }

  /* Post options */
  .post-options {
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px solid var(--border);
    border-radius: 14px; padding: 16px 18px;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.05), 0 1px 2px rgba(0,0,0,0.4);
  }
  .post-label {
    font-size: 10px; font-weight: 600; margin-bottom: 14px;
    color: var(--text2); text-transform: uppercase; letter-spacing: 1.2px;
  }
  .post-field { margin-bottom: 14px; }
  .post-field label, .post-field-label {
    display: block; font-size: 11px; color: var(--text3);
    margin-bottom: 6px; text-transform: uppercase; letter-spacing: 0.6px;
  }
  .post-field select { width: 100%; }

  .se-row { display: flex; gap: 10px; }
  .se-label { display: flex; align-items: center; gap: 6px; font-size: 11px; color: var(--text2); }
  .se-input { width: 70px; padding: 4px 8px; font-size: 12px; }

  .chips-row { display: flex; flex-wrap: wrap; gap: 5px; margin-bottom: 4px; min-height: 24px; align-items: center; }
  .chips-empty { font-size: 11px; color: var(--text3); font-style: italic; }
  .chip {
    display: inline-flex; align-items: center; gap: 5px;
    background: rgba(0, 180, 216, 0.1);
    border: 1px solid rgba(0, 180, 216, 0.3);
    color: var(--blue-hot);
    padding: 4px 10px; font-size: 11px; border-radius: 7px;
    font-weight: 500;
  }
  .chip-unknown {
    background: rgba(255, 214, 10, 0.08);
    border-color: rgba(255, 214, 10, 0.35);
    color: var(--yellow);
  }
  .chip-x {
    background: none; color: inherit; opacity: 0.6;
    padding: 0 2px; font-size: 11px; line-height: 1;
    border-radius: 3px;
  }
  .chip-x:hover { color: var(--red-hot); opacity: 1; }

  .add-row { display: flex; gap: 6px; margin-top: 8px; }
  .add-row select { flex: 1; }
  .btn-add {
    background: rgba(255,255,255,0.04);
    border: 1px solid var(--border);
    color: var(--blue-hot);
    padding: 6px 14px; font-size: 14px; font-weight: 600;
    border-radius: 7px; flex: none;
    transition: all 160ms ease;
  }
  .btn-add:hover {
    background: rgba(0, 180, 216, 0.08);
    border-color: rgba(0, 180, 216, 0.35);
  }

  .upload-types { display: flex; gap: 18px; align-items: center; }
  .btn-full-auto {
    background: rgba(255, 214, 10, 0.08);
    border: 1px solid rgba(255, 214, 10, 0.3);
    color: var(--yellow);
    padding: 6px 14px; font-size: 11px; font-weight: 700;
    border-radius: 8px;
    text-transform: uppercase; letter-spacing: 0.6px;
    transition: all 160ms ease;
  }
  .btn-full-auto:hover { background: rgba(255, 214, 10, 0.15); }
  .btn-full-auto.active {
    background: var(--grad-primary); color: #fff;
    border-color: rgba(0,0,0,0.25);
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.25), 0 6px 18px -6px var(--accent-glow);
  }
  .check-label {
    display: flex; align-items: center; gap: 7px;
    font-size: 13px; color: var(--text); cursor: pointer;
  }
  .check-label input { width: auto; accent-color: var(--red-hot); }

  .post-result {
    margin-top: 10px; padding: 10px 13px;
    border-radius: 10px; font-size: 12px; font-weight: 500;
  }
  .post-result-ok {
    background: rgba(34, 197, 94, 0.08);
    border: 1px solid rgba(34, 197, 94, 0.25);
    color: #7ef0c0;
  }
  .post-result-err {
    background: rgba(239, 68, 68, 0.08);
    border: 1px solid rgba(239, 68, 68, 0.25);
    color: #ff9585;
  }
  .post-result-details { margin-top: 4px; font-size: 11px; opacity: 0.75; font-weight: 400; }

  .post-actions { display: flex; gap: 10px; margin-top: 16px; }
  .btn-start {
    color: #fff; background: var(--grad-primary);
    border: 1px solid rgba(0,0,0,0.25);
    padding: 11px 24px; font-weight: 600;
    letter-spacing: 0.2px; font-size: 13px;
    box-shadow:
      inset 0 1px 0 rgba(255,255,255,0.25),
      inset 0 -1px 0 rgba(0,0,0,0.2),
      0 1px 2px rgba(0,0,0,0.4),
      0 8px 24px -6px var(--accent-glow);
  }
  .btn-start:hover:not(:disabled) {
    background: var(--grad-primary-hover);
    filter: brightness(1.05);
    box-shadow:
      inset 0 1px 0 rgba(255,255,255,0.3),
      inset 0 -1px 0 rgba(0,0,0,0.2),
      0 2px 6px rgba(0,0,0,0.5),
      0 12px 32px -4px var(--accent-glow);
  }
  .btn-start:active:not(:disabled) { transform: translateY(1px); filter: brightness(0.95); }
  .btn-start:disabled { opacity: 0.4; cursor: default; box-shadow: none; }
  .btn-reset {
    background: rgba(255,255,255,0.03);
    color: var(--text2);
    border: 1px solid var(--border);
    padding: 10px 16px;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.04);
  }
  .btn-cancel {
    background: rgba(239, 68, 68, 0.12);
    color: #ff9585;
    border: 1px solid rgba(239, 68, 68, 0.35);
    padding: 10px 16px; font-weight: 600;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.04);
  }
  .btn-cancel:hover {
    background: rgba(239, 68, 68, 0.2);
    border-color: rgba(239, 68, 68, 0.55);
  }
  .btn-reset:hover {
    background: rgba(255,255,255,0.06);
    border-color: rgba(0, 180, 216, 0.35);
    color: var(--text);
  }

  .nzb-info { font-size: 11px; color: var(--text3); margin-bottom: 8px; }

  /* Recherches TMDB + Hydracker côte à côte */
  .searches-row { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
  @media (max-width: 1100px) { .searches-row { grid-template-columns: 1fr; } }

  /* Layout 2-colonnes pour les options de post (gain ~50% de hauteur) */
  .post-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px 24px; }
  .post-grid > .post-field { margin: 0; }

  /* Barres DDL côte à côte — hauteur égale, largeur égale */
  .ddl-bars { display: grid; grid-template-columns: 1fr 1fr; grid-auto-rows: 1fr; gap: 10px; margin-top: 6px; }
  @media (max-width: 900px) {
    .post-grid, .ddl-bars { grid-template-columns: 1fr; grid-auto-rows: auto; }
  }
  .ddl-bar-card {
    background: rgba(0,0,0,0.25);
    border: 1px solid var(--border);
    border-radius: 10px; padding: 10px 12px;
    display: flex; flex-direction: column; gap: 6px;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.03);
    min-width: 0; /* évite l'overflow horizontal */
  }
  .ddl-bar-card.done {
    border-color: rgba(34, 197, 94, 0.3);
    background: rgba(34, 197, 94, 0.05);
  }
  .ddl-bar-card.error {
    border-color: rgba(239, 68, 68, 0.3);
    background: rgba(239, 68, 68, 0.05);
  }
  .ddl-bar-header { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; min-width: 0; }
  .ddl-bar-host {
    font-size: 11px; font-weight: 700; color: var(--blue-hot);
    text-transform: uppercase; letter-spacing: 0.4px; flex: none;
  }
  .ddl-bar-speed { font-size: 11px; color: var(--text3); font-variant-numeric: tabular-nums; margin-left: auto; }
  .ddl-bar-status { font-size: 11px; font-weight: 600; margin-left: auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ddl-bar-status.ok { color: #7ef0c0; }
  .ddl-bar-status.err { color: #ff9585; }
  .ddl-bar-status.skipped { color: #ffe066; }
  .ddl-bar-status.posting { color: var(--yellow); }
  .progress-fill.posting {
    background: linear-gradient(180deg, var(--yellow) 0%, var(--orange) 100%);
    animation: pulse 1s ease-in-out infinite;
  }
  @keyframes pulse { 0%,100% { opacity:1 } 50% { opacity:0.65 } }
  .ddl-bar-filename {
    font-size: 10px; color: var(--text3);
    word-break: break-all; line-height: 1.3;
  }
  .ddl-bar-row { display: flex; align-items: center; gap: 9px; }
  .ddl-bar-pct {
    font-size: 12px; color: var(--text2);
    width: 40px; text-align: right; flex: none;
    font-variant-numeric: tabular-nums;
  }
  .ddl-bar-errmsg { font-size: 11px; color: #ff9585; }
  .ddl-step { display: flex; flex-direction: column; gap: 4px; }
  .ddl-step-label {
    display: flex; justify-content: space-between; align-items: center;
    font-size: 11px; color: var(--text3);
  }
  .progress-fill.hydone {
    background: linear-gradient(180deg, #34d399 0%, #22c55e 100%);
    box-shadow: 0 0 12px rgba(34, 197, 94, 0.4);
  }
  .nzb-progress-block { display: flex; flex-direction: column; gap: 4px; }
  .nzb-step-label { font-size: 11px; color: var(--text2); }
  .progress-bar {
    height: 7px; background: rgba(255,255,255,0.06);
    border-radius: 4px; overflow: hidden;
    box-shadow: inset 0 1px 1px rgba(0,0,0,0.3);
  }
  .progress-fill {
    height: 100%; background: var(--grad-primary);
    border-radius: 4px;
    transition: width 0.12s linear;
    box-shadow: 0 0 10px rgba(230, 57, 70, 0.35);
  }

  /* Récapitulatif (repliable) */
  .recap-box {
    background: linear-gradient(180deg, rgba(0, 180, 216, 0.05) 0%, rgba(0, 180, 216, 0.02) 100%);
    border: 1px solid rgba(0, 180, 216, 0.2);
    border-radius: 14px; overflow: hidden;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.05);
  }
  .recap-title {
    display: flex; align-items: center; gap: 6px; width: 100%;
    background: transparent; border: 0; padding: 10px 18px;
    font-size: 11px; font-weight: 700; color: var(--blue-hot);
    text-transform: uppercase; letter-spacing: 1.2px;
    cursor: pointer; text-align: left;
  }
  .recap-title:hover { background: rgba(255,255,255,0.02); }
  .recap-box.open .recap-title { border-bottom: 1px solid rgba(0, 180, 216, 0.2); }
  .recap-body { padding: 12px 18px 14px; }
  .recap-row {
    display: flex; justify-content: space-between; align-items: baseline;
    padding: 5px 0; border-bottom: 1px solid rgba(255,255,255,0.06);
    font-size: 12px;
  }
  .recap-row:last-child { border-bottom: none; }
  .recap-key {
    color: var(--text3); flex: none; width: 120px;
    text-transform: uppercase; font-size: 10px; letter-spacing: 0.5px;
  }
  .recap-val { color: var(--text); text-align: right; flex: 1; }
  .recap-id { color: var(--red-hot); }
  .recap-type {
    background: rgba(0, 180, 216, 0.15); color: var(--blue-hot);
    font-size: 10px; padding: 1px 7px; border-radius: 9999px; margin-left: 4px;
  }
  .recap-file { color: var(--text2); font-size: 11px; word-break: break-all; }

  .nfo-preview {
    background: rgba(0,0,0,0.3);
    border: 1px solid var(--border);
    border-radius: 12px;
    overflow: hidden;
  }
  .nfo-preview-header {
    display: flex; justify-content: space-between; align-items: center;
    gap: 8px; width: 100%;
    padding: 10px 14px;
    background: transparent; border: 0;
    font-size: 11px; font-weight: 600; color: var(--text2);
    text-transform: uppercase; letter-spacing: 1.2px;
    cursor: pointer; text-align: left;
  }
  .nfo-preview-header:hover { background: rgba(255,255,255,0.03); }
  .nfo-preview.open .nfo-preview-header { border-bottom: 1px solid var(--border); }
  .nfo-preview .nfo-preview-body { margin: 0 14px 14px; }
  .nfo-preview-body {
    margin: 0; padding: 12px;
    background: rgba(0,0,0,0.4);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 11px; line-height: 1.45;
    white-space: pre; overflow-x: auto;
    max-height: 360px; overflow-y: auto;
  }
</style>
