<script>
  import { onMount, onDestroy } from 'svelte'
  import { GetConfig, SaveConfig, TestHydracker, TestTMDB, TestOneFichier, TestSendCm, TestFTP, TestSeedbox, TestQBit, TestModSeedbox, TestUsenet, TestLihdl, HasSeedboxSettingsPassword, SetSeedboxSettingsPassword, VerifySeedboxSettingsPassword, ClearSeedboxSettingsPassword } from '../wailsjs/go/main/App.js'
  import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime.js'
  import HydrackerTab from './HydrackerTab.svelte'
  import { logEntries, addLog, clearLogs } from './logs.js'
  import logo from './assets/logo.png'
  import { ListCheckTorrents, ReseedFromLihdl, ReseedPrepare, ReseedExecute, SelectAnyTorrentFile, SelectMkvFile, GetVersion, StartWatchFolder, StopWatchFolder, IsWatching, CheckForUpdate, OpenBrowser, HistoryList, HistoryDelete, HistoryStats, DownloadUpdate, HasLihdlSettingsPassword, SetLihdlSettingsPassword, VerifyLihdlSettingsPassword, ClearLihdlSettingsPassword, IsLihdlPasswordManaged, IsHydrackerURLManaged, GetEffectiveHydrackerURL, FindHydrackerSources, FicheGetContent, FicheGetNfo, GetDDLFilename, GetUploaderStats, HydrackerSearch, TMDBGetByImdbID, TMDBGetProviders, HydrackerGetByID, HydrackerGetByTmdbID, DownloadToDownloads, AutoReseedFromHydracker, AutoReseedDDLFromHydracker, AutoReseedFullFromTorrent, ListReseedRequests, ListMyLiens, ListMyTorrents, DeleteMyLien, DeleteMyTorrent, DeleteMyNzb, DeleteTorrentAndFTP, ListSeedboxHashes, GetNexumIndex, TestNexum, UpdateMyLien, UpdateMyTorrent, GetMetaQualities, ListTitlesSorted, GetUserProfile, ParseFilename, Notify } from '../wailsjs/go/main/App.js'

  // --- Tabs ---
  const TABS = [
    { id: 'hydracker', label: '🎬 Hydracker' },
    { id: 'fiches',    label: '🎞 Fiches' },
    { id: 'check',     label: '🔍 Check Torrent' },
    { id: 'requests',  label: '📋 Demandes Reseed' },
    { id: 'reseed',    label: '♻️ Reseed' },
    { id: 'myuploads', label: '📤 Mes uploads' },
    { id: 'history',   label: '📚 Historique' },
    { id: 'apilog',    label: '🔬 Log API' },
    { id: 'settings',  label: '⚙️ Réglages' },
    { id: 'log',       label: '📋 Journal' },
  ]
  let activeTab = 'hydracker'

  // --- Config ---
  let cfg = {
    hydracker_token: '',
    tmdb_api_key: '',
    tmdb_proxy_url: 'https://tmdb.uklm.xyz',
    lihdl_base_url: '',
    one_fichier_api_key: '',
    sendcm_api_key: '',
    nexum_api_key: '',
    nexum_base_url: 'https://nexum-core.com',
    usenet_host: '', usenet_port: 119, usenet_ssl: false,
    usenet_user: '', usenet_password: '', usenet_connections: 20,
    usenet_group: 'alt.binaries.test',
    parpar_redundancy: 5, parpar_threads: 8, parpar_slice_size: 768000,
    ftp_host: '', ftp_port: 21, ftp_user: '', ftp_password: '', ftp_path: '',
    private_ftp_host: '', private_ftp_port: 21, private_ftp_user: '', private_ftp_password: '', private_ftp_path: '',
    seedbox_url: '', seedbox_user: '', seedbox_password: '', seedbox_label: '',
    private_seedbox_url: '', private_seedbox_user: '', private_seedbox_password: '', private_seedbox_label: '',
    private_qbit_url: '', private_qbit_user: '', private_qbit_password: '',
    qbit_url: '', qbit_user: '', qbit_password: '',
    mod_seedbox_url: '', mod_seedbox_user: '', mod_seedbox_password: '',
    ftp_mod_host: '', ftp_mod_port: 21, ftp_mod_user: '', ftp_mod_password: '', ftp_mod_path: '/',
    tracker_url: '', torrent_piece_size: 8388608,
    lihdl_user: '', lihdl_password: '',
    watch_folder: '', watch_auto_start: false,
    proxy_url: '',
  }
  let appVersion = ''
  let watchRunning = false
  let updateInfo = null  // { available, current, latest, url }
  let cfgSaved = false
  let sidebarCollapsed = false
  let updateChecking = false
  let updateCheckMsg = ''

  // LiHDL : protection admin (index + recherche TMDB — bonus pour les admins
  // qui ont le mdp partagé, les autres s'en passent et utilisent l'API TMDB).
  let lihdlUnlocked = false
  let lihdlHasPassword = false
  let lihdlManaged = false // true = mdp imposé au build, user ne peut pas le modifier
  let hydrackerURLManaged = false // true = URL API imposée au build
  let lihdlModal = null  // null | 'unlock' | 'create' | 'change' | 'remove'
  let lihdlPwdInput = ''
  let lihdlPwdCurrent = ''
  let lihdlPwdNew = ''
  let lihdlPwdConfirm = ''
  let lihdlPwdError = ''

  async function checkLihdlPasswordStatus() {
    try { lihdlHasPassword = await HasLihdlSettingsPassword() } catch(e) { lihdlHasPassword = false }
    try { lihdlManaged = await IsLihdlPasswordManaged() } catch(e) { lihdlManaged = false }
  }

  // --- Sections SEEDBOX/FTP : plus de mdp, toujours ouvert ---
  let seedboxUnlocked = true
  let seedboxHasPassword = false
  let seedboxModal = null  // null | 'unlock' | 'create' | 'change' | 'remove'
  let seedboxPwdInput = ''
  let seedboxPwdCurrent = ''
  let seedboxPwdNew = ''
  let seedboxPwdConfirm = ''
  let seedboxPwdError = ''

  async function checkSeedboxPasswordStatus() {
    try { seedboxHasPassword = await HasSeedboxSettingsPassword() } catch(e) { seedboxHasPassword = false }
  }

  function openSeedboxLockModal() {
    seedboxPwdInput = ''; seedboxPwdCurrent = ''; seedboxPwdNew = ''; seedboxPwdConfirm = ''; seedboxPwdError = ''
    seedboxModal = seedboxHasPassword ? 'unlock' : 'create'
  }

  async function submitSeedboxPwd() {
    seedboxPwdError = ''
    try {
      if (seedboxModal === 'unlock') {
        const ok = await VerifySeedboxSettingsPassword(seedboxPwdInput)
        if (!ok) { seedboxPwdError = 'Mot de passe incorrect'; return }
        seedboxUnlocked = true
        seedboxModal = null
        // Notifie HydrackerTab que l'user vient d'être confirmé admin
        // → Torrent ADMIN devient visible dans la section Uploader via.
        window.dispatchEvent(new CustomEvent('admin-unlocked'))
      } else if (seedboxModal === 'create') {
        if (seedboxPwdNew.length < 4) { seedboxPwdError = 'Mot de passe trop court (4 min)'; return }
        if (seedboxPwdNew !== seedboxPwdConfirm) { seedboxPwdError = 'Les mots de passe ne correspondent pas'; return }
        await SetSeedboxSettingsPassword('', seedboxPwdNew)
        seedboxHasPassword = true
        seedboxUnlocked = true
        seedboxModal = null
      } else if (seedboxModal === 'change') {
        if (seedboxPwdNew.length < 4) { seedboxPwdError = 'Mot de passe trop court (4 min)'; return }
        if (seedboxPwdNew !== seedboxPwdConfirm) { seedboxPwdError = 'Les mots de passe ne correspondent pas'; return }
        await SetSeedboxSettingsPassword(seedboxPwdCurrent, seedboxPwdNew)
        seedboxModal = null
      } else if (seedboxModal === 'remove') {
        await ClearSeedboxSettingsPassword(seedboxPwdCurrent)
        seedboxHasPassword = false
        seedboxModal = null
      }
    } catch(e) { seedboxPwdError = String(e).replace('Error: ', '') }
  }

  function openLihdlLockModal() {
    lihdlPwdInput = ''; lihdlPwdCurrent = ''; lihdlPwdNew = ''; lihdlPwdConfirm = ''; lihdlPwdError = ''
    lihdlModal = lihdlHasPassword ? 'unlock' : 'create'
  }

  async function submitLihdlPwd() {
    lihdlPwdError = ''
    try {
      if (lihdlModal === 'unlock') {
        const ok = await VerifyLihdlSettingsPassword(lihdlPwdInput)
        if (!ok) { lihdlPwdError = 'Mot de passe incorrect'; return }
        lihdlUnlocked = true
        lihdlModal = null
      } else if (lihdlModal === 'create') {
        if (lihdlPwdNew.length < 4) { lihdlPwdError = 'Mot de passe trop court (4 min)'; return }
        if (lihdlPwdNew !== lihdlPwdConfirm) { lihdlPwdError = 'Les mots de passe ne correspondent pas'; return }
        await SetLihdlSettingsPassword('', lihdlPwdNew)
        lihdlHasPassword = true
        lihdlUnlocked = true
        lihdlModal = null
      } else if (lihdlModal === 'change') {
        if (lihdlPwdNew.length < 4) { lihdlPwdError = 'Mot de passe trop court (4 min)'; return }
        if (lihdlPwdNew !== lihdlPwdConfirm) { lihdlPwdError = 'Les mots de passe ne correspondent pas'; return }
        await SetLihdlSettingsPassword(lihdlPwdCurrent, lihdlPwdNew)
        lihdlModal = null
      } else if (lihdlModal === 'remove') {
        await ClearLihdlSettingsPassword(lihdlPwdCurrent)
        lihdlHasPassword = false
        lihdlModal = null
      }
    } catch(e) { lihdlPwdError = String(e).replace('Error: ', '') }
  }

  async function recheckUpdate() {
    updateChecking = true
    updateCheckMsg = ''
    addLog('UPDATE', '🔄 Vérification des mises à jour…')
    try {
      updateInfo = await CheckForUpdate()
      addLog('UPDATE', `GitHub: v${updateInfo?.latest || '?'} · Local: v${updateInfo?.current || '?'} · Dispo: ${updateInfo?.available}`)
      if (updateInfo?.available) {
        updateCheckMsg = `🆕 v${updateInfo.latest}`
      } else {
        updateCheckMsg = `✓ À jour (v${updateInfo?.current || ''})`
        setTimeout(() => { updateCheckMsg = '' }, 5000)
      }
    } catch(e) {
      updateCheckMsg = '✗ Erreur'
      addLog('UPDATE', `✗ Erreur check maj: ${e}`)
      setTimeout(() => { updateCheckMsg = '' }, 8000)
    }
    updateChecking = false
  }

  // Update modal
  let showUpdateModal = false
  let updateState = { stage: '', msg: '', percent: 0, downloading: false, downloadedPath: '' }

  async function startDownloadUpdate() {
    updateState = { stage: '', msg: '', percent: 0, downloading: true, downloadedPath: '' }
    try {
      const path = await DownloadUpdate()
      updateState = { ...updateState, downloading: false, downloadedPath: path, stage: 'done' }
    } catch(e) {
      updateState = { ...updateState, downloading: false, stage: 'error', msg: String(e) }
      addLog('UPDATE', '✗ ' + e)
    }
  }

  // Historique
  let histEntries = []
  let histFilter = ''      // '' | 'torrent' | 'nzb' | 'ddl'
  let histQuery = ''
  let histStats = { total: 0, ok: 0, error: 0, torrent: 0, nzb: 0, ddl: 0 }
  let histLoading = false

  async function loadHistory() {
    histLoading = true
    try {
      histEntries = await HistoryList(histFilter, histQuery, 0, 500) || []
      histStats = await HistoryStats() || histStats
    } catch(e) { addLog('HIST', '✗ ' + e) }
    histLoading = false
  }
  $: if (activeTab === 'history') loadHistory()

  async function deleteHistEntry(id) {
    if (!confirm('Supprimer cette entrée ?')) return
    try { await HistoryDelete(id); await loadHistory() } catch(e) {}
  }

  function formatDate(iso) {
    if (!iso) return ''
    const d = new Date(iso)
    return d.toLocaleString('fr-FR', { day:'2-digit', month:'2-digit', year:'2-digit', hour:'2-digit', minute:'2-digit' })
  }

  // NZB live status (partagé avec l'onglet NZB)
  let nzbStatus = ''
  let nzbParparPct = 0
  let nzbNyuuPct = 0
  let nzbNyuuSpeed = ''
  let nzbNyuuETA = ''
  let nzbNyuuArticles = ''
  let nzbDone = false
  let nzbResult = null  // { ok, message }

  // Show passwords
  let showPwd = {}

  // Reseed
  let reseedTorrentPath = ''
  let reseedMkvPath = ''
  let reseedPrepLoading = false
  let reseedPrep = null  // { torrent_name, first_file_name, size, info_hash, search, hydracker_fiche }
  let reseedRunning = false
  let reseedStage = ''
  let reseedMsg = ''
  let reseedPct = 0
  let reseedSpeed = 0

  async function reseedPickTorrent() {
    try {
      const p = await SelectAnyTorrentFile()
      if (p) { reseedTorrentPath = p; reseedPrep = null }
    } catch(e) { addLog('RES', '✗ ' + e) }
  }
  async function reseedPickMkv() {
    try {
      const p = await SelectMkvFile()
      if (p) reseedMkvPath = p
    } catch(e) { addLog('RES', '✗ ' + e) }
  }
  async function reseedAnalyze() {
    if (!reseedTorrentPath) return
    reseedPrepLoading = true
    reseedPrep = null
    try { reseedPrep = await ReseedPrepare(reseedTorrentPath) }
    catch(e) { addLog('RES', '✗ analyse: ' + e) }
    reseedPrepLoading = false
  }
  async function reseedConfirm() {
    if (!reseedTorrentPath || !reseedMkvPath) return
    reseedRunning = true
    reseedStage = 'start'; reseedMsg = 'Démarrage…'; reseedPct = 0; reseedSpeed = 0
    try { await ReseedExecute(reseedTorrentPath, reseedMkvPath) }
    catch(e) { addLog('RES', '✗ ' + e) }
    reseedRunning = false
  }
  function reseedReset() {
    reseedTorrentPath = ''; reseedMkvPath = ''; reseedPrep = null
    reseedStage = ''; reseedMsg = ''; reseedPct = 0; reseedSpeed = 0
  }

  // Télécharge un .torrent / NZB directement dans ~/Downloads (via signed URL Hydracker)
  async function downloadFromHydracker(url, name) {
    if (!url) return
    try {
      const path = await DownloadToDownloads(url, name)
      addLog('RES', `⬇ Téléchargé : ${path}`)
      try { Notify('✓ Téléchargement terminé', name) } catch(e) {}
    } catch(e) { addLog('RES', '✗ ' + e) }
  }

  // --- Auto-reseed : workflow standalone (sans .torrent local, juste un ID Hydracker) ---
  let autoReseedInput = ''            // URL Hydracker ou ID brut
  let autoReseedSaison = 0
  let autoReseedEpisode = 0
  let autoReseedLoading = false
  let autoReseedStatus = ''           // dernier message du workflow
  let autoReseedResult = null         // { torrent_id, torrent_name, size_bytes, seedbox_path }
  let autoReseedError = ''

  function parseHydrackerID(input) {
    const s = (input || '').trim()
    if (!s) return 0
    // URL avec /titles/NNN ou /title/NNN
    const m = s.match(/\/titles?\/(\d+)/)
    if (m) return parseInt(m[1])
    // Juste un nombre
    if (/^\d+$/.test(s)) return parseInt(s)
    return 0
  }

  async function launchAutoReseed() {
    autoReseedError = ''
    autoReseedResult = null
    const id = parseHydrackerID(autoReseedInput)
    if (!id) { autoReseedError = 'ID Hydracker invalide — colle une URL /titles/XXX ou un nombre'; return }
    if (!cfg.seedbox_url) { autoReseedError = 'Seedbox non configurée (Réglages)'; return }
    autoReseedLoading = true
    autoReseedStatus = 'Démarrage…'
    addLog('AUTO', `▶ Auto-reseed torrent fiche #${id} saison=${autoReseedSaison} épisode=${autoReseedEpisode}`)
    try {
      const r = await AutoReseedFromHydracker(id, autoReseedSaison || 0, autoReseedEpisode || 0, 0, 0)
      autoReseedResult = r
      addLog('AUTO', `✓ Torrent #${r.torrent_id} ${r.torrent_name} → seedbox OK`)
      try { Notify('✓ Auto-reseed terminé', r.torrent_name) } catch(e) {}
    } catch(e) {
      autoReseedError = String(e?.message || e)
      addLog('AUTO', `✗ ${autoReseedError}`)
    }
    autoReseedLoading = false
  }

  // Variante DDL → FTP : quand la fiche n'a pas de torrent partagé via API mais
  // qu'un DDL 1fichier existe, on télécharge depuis le DDL et on pousse direct
  // sur le FTP configuré (streaming, pas de passage sur le disque local).
  let autoReseedDDLProgress = { percent: 0, speed_mb: 0, bytes: 0, total: 0 }
  async function launchAutoReseedDDL() {
    autoReseedError = ''
    autoReseedResult = null
    autoReseedDDLProgress = { percent: 0, speed_mb: 0, bytes: 0, total: 0 }
    const id = parseHydrackerID(autoReseedInput)
    if (!id) { autoReseedError = 'ID Hydracker invalide — colle une URL /titles/XXX ou un nombre'; return }
    if (!cfg.ftp_host) { autoReseedError = 'FTP non configuré (Réglages)'; return }
    if (!cfg.one_fichier_api_key) { autoReseedError = 'Clé API 1fichier non configurée (Réglages) — requise pour le DDL'; return }
    autoReseedLoading = true
    autoReseedStatus = 'Démarrage DDL…'
    addLog('AUTO', `▶ Auto-reseed DDL fiche #${id} saison=${autoReseedSaison} épisode=${autoReseedEpisode}`)
    try {
      const r = await AutoReseedDDLFromHydracker(id, autoReseedSaison || 0, autoReseedEpisode || 0, 0, 0)
      autoReseedResult = { torrent_id: r.lien_id, torrent_name: r.filename, size_bytes: r.size_bytes, seedbox_path: 'FTP : ' + r.ftp_remote_name + ' (host: ' + r.host + ')' }
      addLog('AUTO', `✓ DDL #${r.lien_id} ${r.filename} → FTP OK`)
      try { Notify('✓ Auto-reseed DDL terminé', r.filename) } catch(e) {}
    } catch(e) {
      autoReseedError = String(e?.message || e)
      addLog('AUTO', `✗ ${autoReseedError}`)
    }
    autoReseedLoading = false
  }

  // Ouvre l'onglet Hydracker avec le .torrent + la fiche Hydracker pré-remplis.
  // Utile quand on a un .torrent existant (sans seedbox) : on saute la phase de
  // création/FTP et on poste directement le fichier à Hydracker.
  function reseedOpenInHydracker(prep) {
    if (!prep) return
    activeTab = 'hydracker'
    window.dispatchEvent(new CustomEvent('hydracker:preload-torrent', { detail: {
      torrentPath: reseedTorrentPath,
      mkvPath: reseedMkvPath || '',
      hydrackerFiche: prep.hydracker_fiche,
      tmdbSearch: prep.search,
      torrentName: prep.torrent_name,
    } }))
  }

// --- Fiches : recherche + drill-down sur toutes les fiches du site ---
  let fichesMode = 'name'        // 'name' | 'hydracker_id' | 'tmdb_id'
  let fichesQuery = ''
  let fichesResults = []          // []PartialTitle
  let fichesLoading = false
  let fichesError = ''
  let fichesSelected = null       // fiche actuellement ouverte en détail
  let fichesContent = null        // {liens, nzbs, torrents} de la fiche sélectionnée
  let fichesContentLoading = false
  let fichesContentTab = 'torrents'  // 'torrents' | 'nzbs' | 'liens'

  async function fichesSearch() {
    fichesError = ''
    fichesResults = []
    fichesSelected = null
    const q = (fichesQuery || '').trim()
    if (!q) return
    fichesLoading = true
    try {
      if (fichesMode === 'name') {
        fichesResults = await HydrackerSearch(q) || []
      } else if (fichesMode === 'hydracker_id') {
        const id = parseInt(q.match(/\d+/)?.[0] || '0')
        if (!id) { fichesError = 'ID Hydracker invalide'; fichesLoading = false; return }
        const t = await HydrackerGetByID(id)
        fichesResults = t ? [t] : []
      } else if (fichesMode === 'tmdb_id') {
        const id = parseInt(q.match(/\d+/)?.[0] || '0')
        if (!id) { fichesError = 'ID TMDB invalide'; fichesLoading = false; return }
        const t = await HydrackerGetByTmdbID(id)
        fichesResults = t ? [t] : []
      }
      if (!fichesResults.length) fichesError = 'Aucune fiche trouvée'
    } catch(e) {
      fichesError = String(e?.message || e)
    }
    fichesLoading = false
  }

  // État bonus pour la fiche détail (note IMDb + watch providers via proxytmdb)
  let fichesProviders = null      // map[country]CountryProviders ou null
  let fichesProvidersLoading = false
  let fichesImdbInfo = null       // tmdb.Movie avec note_imdb

  async function fichesOpen(fiche) {
    fichesSelected = fiche
    fichesContent = null
    fichesContentLoading = true
    fichesContentTab = 'torrents'
    fichesProviders = null
    fichesImdbInfo = null
    try {
      fichesContent = await FicheGetContent(fiche.id)
    } catch(e) {
      addLog('FICHE', '✗ ' + e)
    }
    fichesContentLoading = false

    // Bonus async (non-bloquant) : fetch watch providers + note IMDb
    if (fiche.tmdb_id) {
      const mediaType = fiche.type === 'series' ? 'tv' : 'movie'
      fichesProvidersLoading = true
      TMDBGetProviders(fiche.tmdb_id, mediaType)
        .then(p => { fichesProviders = p || {} })
        .catch(() => {})
        .finally(() => { fichesProvidersLoading = false })
    }
    if (fiche.imdb_id) {
      TMDBGetByImdbID(fiche.imdb_id)
        .then(m => { fichesImdbInfo = m })
        .catch(() => {})
    }
  }

  function fichesBackToResults() {
    fichesSelected = null
    fichesContent = null
    fichesProviders = null
    fichesImdbInfo = null
  }

  async function downloadLienFromFiche(lien) {
    try {
      addLog('FICHE', `⬇ Téléchargement lien #${lien.id}…`)
      await DownloadToDownloads(lien.download_url || lien.lien || '', (lien.name || 'hydracker-lien-' + lien.id))
      addLog('FICHE', `✓ Lien #${lien.id} téléchargé dans Downloads`)
    } catch(e) { addLog('FICHE', '✗ ' + e) }
  }

  // --- Modale NFO ---
  let nfoModal = null  // { kind, id, title, html, loading, error }
  async function openNfo(kind, id, title) {
    nfoModal = { kind, id, title, html: '', loading: true, error: '' }
    try {
      const html = await FicheGetNfo(kind, id)
      if (!html || !html.trim()) {
        nfoModal = { ...nfoModal, loading: false, error: 'Aucun NFO disponible pour cet item.' }
      } else {
        nfoModal = { ...nfoModal, loading: false, html }
      }
    } catch(e) {
      nfoModal = { ...nfoModal, loading: false, error: String(e) }
    }
  }
  function closeNfo() { nfoModal = null }

  // --- DDL filename resolver (cache mémoire pendant la session) ---
  // ddlFilenames[lien.id] = { state: 'loading'|'ok'|'err', filename, error }
  // Queue séquentielle avec délai pour éviter rate limit 1fichier (429).
  let ddlFilenames = {}
  let ddlQueue = []
  let ddlQueueRunning = false
  async function processDDLQueue() {
    if (ddlQueueRunning) return
    ddlQueueRunning = true
    while (ddlQueue.length > 0) {
      const { lienId, url } = ddlQueue.shift()
      // Retry simple : 1 retry après 2s sur 429, sinon abandon
      let attempt = 0
      while (attempt < 2) {
        try {
          const fname = await GetDDLFilename(url)
          ddlFilenames = { ...ddlFilenames, [lienId]: { state: 'ok', filename: fname } }
          break
        } catch(e) {
          const msg = String(e)
          if (attempt === 0 && /429|too many/i.test(msg)) {
            attempt++
            await new Promise(r => setTimeout(r, 2500))  // backoff
            continue
          }
          ddlFilenames = { ...ddlFilenames, [lienId]: { state: 'err', error: msg } }
          break
        }
      }
      // Délai entre chaque requête pour rester sous le rate limit
      await new Promise(r => setTimeout(r, 350))
    }
    ddlQueueRunning = false
  }
  function resolveDDLName(lienId, url) {
    if (!url || !url.includes('1fichier.com')) return
    if (ddlFilenames[lienId]) return  // déjà résolu/en cours/échoué
    if (ddlQueue.some(x => x.lienId === lienId)) return  // déjà en queue
    ddlFilenames = { ...ddlFilenames, [lienId]: { state: 'loading' } }
    ddlQueue.push({ lienId, url })
    processDDLQueue()
  }
  // Auto-résolution batch dès qu'on bascule sur l'onglet "Liens DDL" d'une fiche.
  // Limité à 1fichier pour l'instant. Les autres hosts restent affichés sans nom.
  $: if (fichesContentTab === 'liens' && fichesContent?.liens?.liens?.length) {
    for (const l of fichesContent.liens.liens) {
      if (l.lien && l.lien.includes('1fichier.com')) resolveDDLName(l.id, l.lien)
    }
  }

  // Formatters partagés pour les cartes Fiches
  function fmtSize(b) {
    if (!b) return ''
    if (b >= 1e9) return (b/1e9).toFixed(2) + ' GB'
    if (b >= 1e6) return (b/1e6).toFixed(0) + ' MB'
    return (b/1e3).toFixed(0) + ' KB'
  }
  // Drapeaux pour les langues les plus courantes
  const LANG_FLAGS = {
    'TrueFrench': '🇫🇷', 'French': '🇫🇷', 'French (Canada)': '🇨🇦', 'FRENCH AD': '🇫🇷👁',
    'English': '🇬🇧', 'Spanish': '🇪🇸', 'German': '🇩🇪', 'Italian': '🇮🇹',
    'Japanese': '🇯🇵', 'Korean': '🇰🇷', 'Chinese': '🇨🇳', 'Portuguese': '🇵🇹',
    'Dutch': '🇳🇱', 'Russian': '🇷🇺', 'Arabic': '🇸🇦', 'Hindi': '🇮🇳',
  }
  function langFlag(name) { return LANG_FLAGS[name] || '🌐' }

// --- Check Torrent : nouvelle vue "Mes seeds Hydracker" ---
  let mySeedsTorrents = []          // []TorrentItem depuis /admin/torrents?author=Gandalf
  let mySeedsLoading = false
  let mySeedsError = ''
  let mySeedsPage = 1
  let mySeedsTotalPages = 1
  let mySeedsFilter = 'all'         // 'all' | '0seed' | 'low' | 'ok'
  let mySeedsActioning = {}         // { [torrentId]: true } pendant auto-reseed
  let checkActionProg = {}          // { [torrentId]: {stage, percent, speed_mb, msg} }
  let checkActionActiveID = 0       // ID du torrent en cours d'action (pour router les events)

  // Bouton "Parcourir local" : check si un MKV local correspond à une fiche Hydracker
  let localMkvCheck = null          // { filename, parsed, hydrackerFiche, content, error }
  let localMkvChecking = false

  // Set des hashes que l'user a actuellement sur sa seedbox (lowercase).
  // Rempli au load via ListSeedboxHashes — sert à filtrer Check Torrent pour
  // ne montrer que les torrents qu'on a réellement encore en seed.
  let seedboxHashes = new Set()
  let seedboxHashesLoaded = false
  let onlyMine = true   // toggle "présents seulement" / "tous les Hydracker"

  // Index Nexum : map lowercase info_hash → {id, name, seeders, created_at, …}
  let nexumIndex = {}
  let nexumLoaded = false

  async function loadMySeeds() {
    mySeedsLoading = true
    mySeedsError = ''
    try {
      const r = await ListMyTorrents(myUsername, mySeedsPage)
      mySeedsTorrents = r?.pagination?.data || []
      mySeedsTotalPages = r?.pagination?.last_page || 1
      // Tri local par seeders ASC pour mettre les "à risque" en haut
      mySeedsTorrents.sort((a, b) => (a.seeders || 0) - (b.seeders || 0))
    } catch(e) {
      mySeedsError = String(e?.message || e)
      addLog('SEED', '✗ ' + mySeedsError)
    }
    mySeedsLoading = false
    loadSeedboxHashes()
  }

  async function loadSeedboxHashes() {
    try {
      const list = await ListSeedboxHashes()
      seedboxHashes = new Set((list || []).map(h => h.toLowerCase()))
      seedboxHashesLoaded = true
      addLog('SEED', `📡 Seedbox : ${seedboxHashes.size} torrents trouvés sur ta seedbox`)
    } catch(e) {
      seedboxHashesLoaded = false
      addLog('SEED', `⚠ Liste seedbox indispo : ${e}`)
    }
  }

  $: filteredMySeeds = mySeedsTorrents.filter(t => {
    // Filtre 1 : présent sur ta seedbox (si toggle activé et liste chargée)
    if (onlyMine && seedboxHashesLoaded) {
      const h = (t.info_hash || t.hash || '').toLowerCase()
      if (!h || !seedboxHashes.has(h)) return false
    }
    // Filtre 2 : seeders count
    const s = t.seeders || 0
    if (mySeedsFilter === '0seed') return s === 0
    if (mySeedsFilter === 'low') return s > 0 && s <= 2
    if (mySeedsFilter === 'ok') return s >= 3
    return true
  })

  $: if (activeTab === 'check') loadMySeeds()

  async function autoReseedFromCheck(t) {
    if (!t?.title_id) return
    mySeedsActioning = { ...mySeedsActioning, [t.id]: true }
    checkActionActiveID = t.id
    checkActionProg = { ...checkActionProg, [t.id]: { stage: 'start', percent: 0, speed_mb: 0, msg: 'Upload .torrent…' } }
    addLog('SEED', `▶ Torrent → seedbox #${t.id} (fiche #${t.title_id})`)
    try {
      const r = await AutoReseedFromHydracker(t.title_id, t.saison || 0, t.episode || 0, 0, 0)
      checkActionProg = { ...checkActionProg, [t.id]: { stage: 'done', percent: 100, speed_mb: 0, msg: 'Terminé — recheck OK' } }
      addLog('SEED', `✓ Reseed OK → ${r.torrent_name}`)
      try { Notify('✓ Reseed OK', t.torrent_name || t.name) } catch(e) {}
      setTimeout(loadMySeeds, 1500)
    } catch(e) {
      checkActionProg = { ...checkActionProg, [t.id]: { stage: 'error', percent: 0, speed_mb: 0, msg: String(e) } }
      addLog('SEED', `✗ Reseed #${t.id} : ${e}`)
    }
    mySeedsActioning = { ...mySeedsActioning, [t.id]: false }
    checkActionActiveID = 0
    setTimeout(() => { const { [t.id]: _, ...rest } = checkActionProg; checkActionProg = rest }, 4000)
  }

  // Reseed complet : DDL 1fichier → FTP (nom exact torrent) + .torrent seedbox + recheck
  async function fullReseedFromCheck(t) {
    if (!t?.title_id || !t?.id) return
    mySeedsActioning = { ...mySeedsActioning, [t.id]: true }
    checkActionActiveID = t.id
    checkActionProg = { ...checkActionProg, [t.id]: { stage: 'start', percent: 0, speed_mb: 0, msg: 'DDL 1fichier…' } }
    addLog('SEED', `▶ Reseed complet torrent #${t.id} (DDL→FTP+seedbox+recheck)`)
    try {
      const r = await AutoReseedFullFromTorrent(t.id, t.title_id, t.saison || 0, t.episode || 0)
      checkActionProg = { ...checkActionProg, [t.id]: { stage: 'done', percent: 100, speed_mb: 0, msg: `Terminé — ${r.rechecked ? 'recheck OK' : 'sans recheck'}` } }
      addLog('SEED', `✓ ${r.expected_filename} → FTP OK · seedbox ${r.seedbox_path}${r.rechecked ? ' · recheck OK' : ''}`)
      try { Notify('✓ Reseed complet OK', r.expected_filename) } catch(e) {}
      setTimeout(loadMySeeds, 1500)
    } catch(e) {
      checkActionProg = { ...checkActionProg, [t.id]: { stage: 'error', percent: 0, speed_mb: 0, msg: String(e) } }
      addLog('SEED', `✗ Reseed complet #${t.id} : ${e}`)
    }
    mySeedsActioning = { ...mySeedsActioning, [t.id]: false }
    checkActionActiveID = 0
    setTimeout(() => { const { [t.id]: _, ...rest } = checkActionProg; checkActionProg = rest }, 4000)
  }

  // --- Suppression complète d'un torrent (Hydracker + FTP) ---
  let deleteTorrentModal = null   // null | { torrent, loading, result, error }
  function askDeleteTorrent(t) {
    deleteTorrentModal = { torrent: t, loading: false, result: null, error: '' }
  }
  async function confirmDeleteTorrent() {
    if (!deleteTorrentModal?.torrent) return
    const t = deleteTorrentModal.torrent
    deleteTorrentModal = { ...deleteTorrentModal, loading: true, error: '' }
    mySeedsActioning = { ...mySeedsActioning, [t.id]: true }
    addLog('DEL', `▶ Suppression torrent #${t.id} + fichier(s) FTP`)
    try {
      const r = await DeleteTorrentAndFTP(t.id)
      deleteTorrentModal = { ...deleteTorrentModal, loading: false, result: r }
      addLog('DEL', `✓ Hydracker OK · FTP ${r.used_ftp || 'rien trouvé'} · ${r.ftp_deleted?.length || 0} fichier(s) supprimé(s)`)
      try { Notify('🗑 Torrent supprimé', t.torrent_name || t.name || ('#' + t.id)) } catch(e) {}
      setTimeout(loadMySeeds, 1500)
    } catch(e) {
      deleteTorrentModal = { ...deleteTorrentModal, loading: false, error: String(e?.message || e) }
      addLog('DEL', `✗ Suppr #${t.id} : ${e}`)
    }
    mySeedsActioning = { ...mySeedsActioning, [t.id]: false }
  }
  function closeDeleteTorrentModal() { deleteTorrentModal = null }

  async function checkLocalMkv() {
    try {
      const path = await SelectMkvFile()
      if (!path) return
      const filename = path.split(/[\\/]/).pop()
      localMkvChecking = true
      localMkvCheck = { filename, parsed: null, hydrackerFiche: null, content: null, error: '' }

      // 1. Parser le nom
      const parsed = await ParseFilename(filename)
      localMkvCheck.parsed = parsed

      // 2. Cherche la fiche Hydracker par titre
      if (parsed?.title) {
        const results = await HydrackerSearch(parsed.title)
        if (results?.length > 0) {
          localMkvCheck.hydrackerFiche = results[0]
          // 3. Récupère le contenu de la fiche (torrents + nzbs + liens)
          localMkvCheck.content = await FicheGetContent(results[0].id)
        } else {
          localMkvCheck.error = 'Aucune fiche Hydracker trouvée pour "' + parsed.title + '"'
        }
      } else {
        localMkvCheck.error = 'Pas de titre extrait du nom de fichier'
      }
    } catch(e) {
      if (localMkvCheck) localMkvCheck.error = String(e?.message || e)
    }
    localMkvChecking = false
  }

  function closeLocalMkvCheck() { localMkvCheck = null }

// --- Demandes de reseed (admin) ---
  let reqFilter = 'pending'        // 'pending' | 'done' | 'rejected' | 'all' | 'mine'
  let reqList = []                  // []ReseedRequest
  let reqLoading = false
  let reqError = ''
  let reqPage = 1
  let reqTotalPages = 1
  let reqProcessing = {}            // { [requestId]: true }
  let reqProgress = {}              // { [requestId]: { stage, msg, percent, speed_mb, bytes, total } }
  let reqActiveID = 0               // ID de la demande en cours (pour router les events)
  let myUserID = 550257             // TODO: récupérer via GetMyUsername/user-profile/me

  async function loadReseedRequests() {
    reqLoading = true
    reqError = ''
    try {
      const status = (reqFilter === 'all' || reqFilter === 'mine') ? '' : reqFilter
      const uploader = reqFilter === 'mine' ? myUserID : 0
      const r = await ListReseedRequests(status, uploader, 0, reqPage)
      reqList = r?.pagination?.data || []
      reqTotalPages = r?.pagination?.last_page || 1
    } catch(e) {
      reqError = String(e?.message || e)
      addLog('REQ', '✗ ' + reqError)
    }
    reqLoading = false
  }

  $: if (activeTab === 'requests') loadReseedRequests()
  // Reset page quand filtre change
  $: if (reqFilter) { reqPage = 1 }

  async function processReseedRequest(req) {
    if (!req?.torrent?.title_id || !req?.torrent_id) { addLog('REQ', '⚠ request #' + req.id + ' : title_id/torrent_id manquant'); return }
    reqProcessing = { ...reqProcessing, [req.id]: true }
    reqActiveID = req.id
    reqProgress = { ...reqProgress, [req.id]: { stage: 'starting', msg: 'Démarrage…', percent: 0, speed_mb: 0, bytes: 0, total: 0 } }
    addLog('REQ', `▶ Traitement request #${req.id} → fiche #${req.torrent.title_id} torrent #${req.torrent_id}`)
    try {
      const saison = parseInt(req.torrent.torrent_name?.match(/[sS](\d{1,2})[eE]/)?.[1] || '0') || 0
      const ep = parseInt(req.torrent.torrent_name?.match(/[sS]\d{1,2}[eE](\d{1,3})/)?.[1] || '0') || 0
      // Workflow complet : DDL 1fichier → FTP (avec nom exact du torrent) +
      // push .torrent seedbox + force recheck. Nécessite FTP + seedbox + 1fichier.
      const r = await AutoReseedFullFromTorrent(req.torrent_id, req.torrent.title_id, saison, ep)
      addLog('REQ', `✓ Request #${req.id} : FTP ${r.expected_filename} · seedbox ${r.seedbox_path}${r.rechecked ? ' · recheck OK' : ' · recheck à refaire manuellement'}`)
      reqProgress = { ...reqProgress, [req.id]: { ...reqProgress[req.id], stage: 'done', msg: '✓ Terminé', percent: 100 } }
      try { Notify('✓ Reseed demandé traité', req.torrent.torrent_name) } catch(e) {}
    } catch(e) {
      // Fallback push torrent seul si workflow complet échoue
      addLog('REQ', `⚠ Reseed complet #${req.id} échoué (${e}) — fallback push torrent seul`)
      try {
        const saison = parseInt(req.torrent.torrent_name?.match(/[sS](\d{1,2})[eE]/)?.[1] || '0') || 0
        const ep = parseInt(req.torrent.torrent_name?.match(/[sS]\d{1,2}[eE](\d{1,3})/)?.[1] || '0') || 0
        const r2 = await AutoReseedFromHydracker(req.torrent.title_id, saison, ep, 0, 0)
        addLog('REQ', `✓ Fallback request #${req.id} : torrent #${r2.torrent_id} → seedbox`)
        reqProgress = { ...reqProgress, [req.id]: { ...reqProgress[req.id], stage: 'done', msg: '✓ Fallback seedbox OK', percent: 100 } }
      } catch(e2) {
        addLog('REQ', `✗ Request #${req.id} : ${e2}`)
        reqProgress = { ...reqProgress, [req.id]: { ...reqProgress[req.id], stage: 'error', msg: '✗ ' + String(e2) } }
      }
    }
    reqProcessing = { ...reqProcessing, [req.id]: false }
    if (reqActiveID === req.id) reqActiveID = 0
  }

  async function processAllPending() {
    const pending = reqList.filter(r => r.status === 'pending')
    addLog('REQ', `▶ Batch : ${pending.length} demande(s)`)
    for (const r of pending) {
      await processReseedRequest(r)
    }
    addLog('REQ', '✓ Batch terminé')
  }

// --- Mes uploads (admin CRUD sur tes propres items) ---
  let myUsername = 'Gandalf'              // TODO: fetch depuis /user-profile/me
  let myTab = 'torrents'                   // 'torrents' | 'liens'
  let myItems = []                         // []Lien ou []TorrentItem
  let myLoading = false
  let myError = ''
  let myPage = 1
  let myTotalPages = 1
  let editingItem = null                   // item en cours d'édition (modal)
  let editPayload = { quality: 0, lang: 0, saison: 0, episode: 0, active: -1 }
  let deletingItem = null                  // item dont on demande confirmation delete
  let deleteConfirmInput = ''              // texte tapé par user pour confirmer
  let qualityOptions = []

  async function loadMyUploads() {
    myLoading = true
    myError = ''
    myItems = []
    try {
      if (myTab === 'torrents') {
        const r = await ListMyTorrents(myUsername, myPage)
        myItems = r?.pagination?.data || []
        myTotalPages = r?.pagination?.last_page || 1
      } else if (myTab === 'liens') {
        const r = await ListMyLiens(myUsername, myPage)
        myItems = r?.pagination?.data || []
        myTotalPages = r?.pagination?.last_page || 1
      }
    } catch(e) {
      myError = String(e?.message || e)
      addLog('MY', '✗ ' + myError)
    }
    myLoading = false
  }

  $: if (activeTab === 'myuploads') { loadMyUploads(); if (!qualityOptions.length) GetMetaQualities().then(q => qualityOptions = q || []).catch(() => {}) }
  $: if (myTab) { myPage = 1 }

  function startEdit(item) {
    editingItem = item
    editPayload = {
      quality: item.qualite || item.quality || 0,
      lang: item.lang_id || item.lang || (item.langues_compact?.[0]?.id) || 0,
      saison: item.saison || item.season || 0,
      episode: item.episode || 0,
      active: item.active === 0 ? 0 : (item.active === 1 ? 1 : -1),
    }
  }
  function cancelEdit() { editingItem = null }

  async function saveEdit() {
    if (!editingItem) return
    const id = editingItem.id
    try {
      if (myTab === 'torrents') {
        await UpdateMyTorrent(id, editPayload.quality, editPayload.lang, editPayload.saison, editPayload.episode, editPayload.active)
      } else {
        await UpdateMyLien(id, editPayload.quality, editPayload.lang, editPayload.saison, editPayload.episode, editPayload.active)
      }
      addLog('MY', `✓ Modifié ${myTab.slice(0,-1)} #${id}`)
      try { Notify('✓ Modifié', `${myTab.slice(0,-1)} #${id}`) } catch(e) {}
      editingItem = null
      loadMyUploads()
    } catch(e) {
      addLog('MY', `✗ ${e}`)
    }
  }

  function startDelete(item) {
    deletingItem = item
    deleteConfirmInput = ''
  }
  function cancelDelete() { deletingItem = null; deleteConfirmInput = '' }

  async function confirmDelete() {
    if (!deletingItem) return
    // Vérif : l'user doit avoir tapé l'ID exact pour confirmer
    if (deleteConfirmInput.trim() !== String(deletingItem.id)) {
      addLog('MY', '⚠ Suppression annulée : ID ne correspond pas')
      return
    }
    const id = deletingItem.id
    try {
      if (myTab === 'torrents') {
        await DeleteMyTorrent(id)
      } else if (myTab === 'liens') {
        await DeleteMyLien(id)
      }
      addLog('MY', `✓ Supprimé ${myTab.slice(0,-1)} #${id}`)
      try { Notify('🗑 Supprimé', `${myTab.slice(0,-1)} #${id}`) } catch(e) {}
      deletingItem = null
      deleteConfirmInput = ''
      loadMyUploads()
    } catch(e) {
      addLog('MY', `✗ ${e}`)
    }
  }

  function qualityName(id) { return qualityOptions.find(q => q.id === id)?.name || ('qual#' + id) }

// --- Stats (global site + fiches + uploaders + user search) ---
  let statsLoading = false
  let statsGlobal = null       // { topTitles, liensSample, torrentsSample, uploaders, qualDist, hostDist }
  let statsError = ''

  // Recherche user : affiche profil + aperçu uploads
  let statsUserQuery = ''
  let statsUserProfile = null
  let statsUserLiens = []
  let statsUserTorrents = []
  let statsUserLoading = false
  let statsUserError = ''

  async function loadGlobalStats() {
    statsLoading = true
    statsError = ''
    try {
      // 1. Top fiches par popularité
      const topTitles = await ListTitlesSorted('popularity:desc', 20, 1).catch(() => null)
      // 2. Sample des 100 derniers liens + torrents pour stats distribution
      const liensSample = await ListMyLiens('', 1).catch(() => null)   // sans filtre user = tous
      const torrentsSample = await ListMyTorrents('', 1).catch(() => null)

      // Agrégation locale
      const liens = liensSample?.pagination?.data || []
      const torrents = torrentsSample?.pagination?.data || []

      // Top uploaders : count par id_user sur les samples combinés
      const uploaderCount = {}
      liens.forEach(l => {
        const u = l.id_user || '?'
        uploaderCount[u] = (uploaderCount[u] || 0) + 1
      })
      torrents.forEach(t => {
        const u = t.author || t.id_user || '?'
        uploaderCount[u] = (uploaderCount[u] || 0) + 1
      })
      const topUploaders = Object.entries(uploaderCount)
        .sort((a, b) => b[1] - a[1])
        .slice(0, 15)
        .map(([username, count]) => ({ username, count }))

      // Répartition qualité (liens + torrents)
      const qualCount = {}
      ;[...liens, ...torrents].forEach(item => {
        const q = item.qualite || item.quality || 0
        if (q) qualCount[q] = (qualCount[q] || 0) + 1
      })
      const qualDist = Object.entries(qualCount)
        .sort((a, b) => b[1] - a[1])
        .map(([qid, count]) => ({ id: parseInt(qid), name: qualityName(parseInt(qid)), count }))

      // Répartition hosts (liens only)
      const hostCount = {}
      liens.forEach(l => {
        const h = l.host?.name || ('#' + (l.id_host || '?'))
        hostCount[h] = (hostCount[h] || 0) + 1
      })
      const hostDist = Object.entries(hostCount)
        .sort((a, b) => b[1] - a[1])
        .map(([name, count]) => ({ name, count }))

      statsGlobal = {
        topTitles: topTitles?.data || [],
        totalTitles: topTitles?.pagination?.total || 0,
        totalLiens: liensSample?.pagination?.total || 0,
        totalTorrents: torrentsSample?.pagination?.total || 0,
        liensCount: liens.length,
        torrentsCount: torrents.length,
        topUploaders,
        qualDist,
        hostDist,
      }
    } catch(e) {
      statsError = String(e?.message || e)
    }
    statsLoading = false
  }

  async function searchStatsUser() {
    statsUserError = ''
    statsUserProfile = null
    statsUserLiens = []
    statsUserTorrents = []
    const q = (statsUserQuery || '').trim()
    if (!q) return
    statsUserLoading = true
    try {
      statsUserProfile = await GetUserProfile(q)
      // Aperçu uploads du user
      const [liens, torrents] = await Promise.all([
        ListMyLiens(q, 1).catch(() => null),
        ListMyTorrents(q, 1).catch(() => null),
      ])
      statsUserLiens = liens?.pagination?.data || []
      statsUserTorrents = torrents?.pagination?.data || []
      statsUserProfile._totalLiens = liens?.pagination?.total || 0
      statsUserProfile._totalTorrents = torrents?.pagination?.total || 0
    } catch(e) {
      statsUserError = String(e?.message || e)
    }
    statsUserLoading = false
  }

  $: if (activeTab === 'stats' && !statsGlobal) loadGlobalStats()
  $: if (activeTab === 'stats' && !qualityOptions.length) GetMetaQualities().then(q => qualityOptions = q || []).catch(() => {})

  // --- Stats uploaders (page d'accueil Stats) ---
  let topUploaders = null          // UploaderScanResult
  let topUploadersLoading = false
  let topUploadersError = ''
  let scanSize = 30  // jours à scanner (au lieu de "fiches")
  let uploadersSort = 'total'      // 'total' | 'torrents' | 'nzbs' | 'liens' | 'size' | 'last' | 'author'
  let uploadersSortDir = 'desc'    // 'desc' | 'asc'
  let uploadersFilter = ''
  async function loadTopUploaders() {
    topUploadersLoading = true
    topUploadersError = ''
    try {
      topUploaders = await GetUploaderStats(scanSize)
    } catch(e) {
      topUploadersError = String(e?.message || e)
    }
    topUploadersLoading = false
  }
  function setSort(col) {
    if (uploadersSort === col) { uploadersSortDir = uploadersSortDir === 'desc' ? 'asc' : 'desc' }
    else { uploadersSort = col; uploadersSortDir = 'desc' }
  }
  $: filteredUploaders = (() => {
    if (!topUploaders?.uploaders) return []
    const q = uploadersFilter.trim().toLowerCase()
    let list = q ? topUploaders.uploaders.filter(u => u.author.toLowerCase().includes(q)) : [...topUploaders.uploaders]
    const dir = uploadersSortDir === 'desc' ? -1 : 1
    const cmp = {
      total:    (a,b) => (a.total - b.total) * dir,
      torrents: (a,b) => (a.torrents - b.torrents) * dir,
      nzbs:     (a,b) => (a.nzbs - b.nzbs) * dir,
      liens:    (a,b) => (a.liens - b.liens) * dir,
      size:     (a,b) => (a.total_size - b.total_size) * dir,
      last:     (a,b) => ((a.last_upload_at||'') > (b.last_upload_at||'') ? 1 : -1) * dir,
      author:   (a,b) => a.author.localeCompare(b.author) * dir,
    }[uploadersSort]
    if (cmp) list.sort(cmp)
    return list
  })()

  function fmtBytes(n) {
    if (!n) return '0'
    if (n > 1e12) return (n/1e12).toFixed(2) + ' TB'
    if (n > 1e9) return (n/1e9).toFixed(2) + ' GB'
    if (n > 1e6) return (n/1e6).toFixed(2) + ' MB'
    return n + ' B'
  }

// --- Log API : capture les entrées api:log émises par le client Go ---
  let apiLogs = []                  // []APILogEntry (max 500 entries, FIFO)
  let apiLogFilter = 'all'          // 'all' | 'ok' | 'error'
  let apiLogSearch = ''
  let apiLogExpanded = {}           // { [idx]: true } pour afficher le body_preview
  const API_LOG_MAX = 500

  function clearApiLogs() { apiLogs = []; apiLogExpanded = {} }

  $: filteredApiLogs = apiLogs.filter(e => {
    if (apiLogFilter === 'ok' && (e.error || e.status >= 400)) return false
    if (apiLogFilter === 'error' && !e.error && e.status < 400) return false
    if (apiLogSearch) {
      const q = apiLogSearch.toLowerCase()
      if (!e.url?.toLowerCase().includes(q) && !e.body_preview?.toLowerCase().includes(q) && !e.error?.toLowerCase().includes(q)) return false
    }
    return true
  })

// Check Torrent
  let checkTorrents = []
  let checkLoading = false
  let checkState = {}  // { [hash]: { stage, msg, percent, speed } }
  let checkFilter = 'all'  // 'all' | 'active' | 'inactive'
  function isIncomplete(t) { return t.size > 0 && t.done < t.size }
  $: filteredCheckTorrents = checkTorrents.filter(t => {
    if (checkFilter === 'active')   return t.is_active === 1
    if (checkFilter === 'inactive') return isIncomplete(t)
    return true
  })

  async function loadCheckTorrents(refresh = false) {
    checkLoading = true
    try { checkTorrents = await ListCheckTorrents(refresh) || [] }
    catch(e) { addLog('CHK', '✗ ' + e) }
    checkLoading = false
  }
  async function reseedOne(t) {
    if (!t.lihdl_url || !t.file_name) return
    checkState = { ...checkState, [t.hash]: { stage: 'download', msg: 'Démarrage…', percent: 0, speed: 0 } }
    try { await ReseedFromLihdl(t.hash, t.lihdl_url, t.file_name) }
    catch(e) { addLog('CHK', '✗ ' + e) }
  }

  onMount(async () => {
    try {
      const loaded = await GetConfig()
      if (loaded) cfg = { ...cfg, ...loaded }
    } catch {}
    try { appVersion = await GetVersion() } catch {}
    try { updateInfo = await CheckForUpdate() } catch {}
    checkLihdlPasswordStatus()
    checkSeedboxPasswordStatus()
    try { hydrackerURLManaged = await IsHydrackerURLManaged() } catch {}
    if (hydrackerURLManaged) {
      try { cfg.hydracker_base_url = await GetEffectiveHydrackerURL() } catch {}
    }
    EventsOn('watch:status', s => { watchRunning = !!s.running; if (s.running) addLog('WATCH', `Surveillance active : ${s.folder}`) })
    EventsOn('update:progress', p => { updateState = { ...updateState, stage: p.stage || '', msg: p.msg || '', percent: p.percent || 0 } })
    EventsOn('watch:newfile', path => {
      addLog('WATCH', `📥 Nouveau fichier : ${path}`)
      window.dispatchEvent(new CustomEvent('watch:newfile', { detail: path }))
    })
    // Démarrage auto si configuré
    if (cfg.watch_auto_start && cfg.watch_folder) {
      try { await StartWatchFolder(cfg.watch_folder) } catch(e) { addLog('WATCH', '✗ ' + e) }
    }
    EventsOn('nzb:status', s => {
      nzbStatus = s
      if (s === 'Terminé') nzbDone = true
      addLog('NZB', s)
    })
    EventsOn('nzb:parpar', p => { if (p.percent !== undefined) nzbParparPct = p.percent })
    EventsOn('nzb:nyuu',   p => {
      if (p.percent  !== undefined) nzbNyuuPct     = p.percent
      if (p.speed    !== undefined) nzbNyuuSpeed   = p.speed
      if (p.eta      !== undefined) nzbNyuuETA     = p.eta
      if (p.articles !== undefined) nzbNyuuArticles = p.articles
    })
    EventsOn('nzb:result', r => { nzbResult = r; nzbDone = true; addLog('NZB', r.message) })
    EventsOn('ddl:log',    msg => addLog('DDL', msg))
    EventsOn('check:log',  msg => addLog('CHK', msg))
    EventsOn('check:status', p => {
      if (!p.hash) return
      checkState = { ...checkState, [p.hash]: { ...(checkState[p.hash] || {}), stage: p.stage, msg: p.msg } }
      addLog('CHK', p.msg)
    })
    EventsOn('check:progress', p => {
      if (!p.hash) return
      checkState = { ...checkState, [p.hash]: { ...(checkState[p.hash] || {}), percent: p.percent ?? 0, speed: p.speed ?? 0 } }
    })
    EventsOn('reseed:status', p => {
      reseedStage = p.stage || ''
      reseedMsg = p.msg || ''
      addLog('RES', p.msg || p.stage)
    })
    EventsOn('autoreseed:status', p => {
      autoReseedStatus = p.msg || p.stage || ''
      if (p.msg) addLog('AUTO', p.msg)
      // Route l'état vers le torrent actif de l'onglet Check Torrent
      if (checkActionActiveID) {
        checkActionProg = { ...checkActionProg, [checkActionActiveID]: {
          ...(checkActionProg[checkActionActiveID] || {}),
          stage: p.stage || 'progress',
          msg: p.msg || '',
        } }
      }
    })
    EventsOn('autoreseed:progress', p => {
      if (checkActionActiveID) {
        checkActionProg = { ...checkActionProg, [checkActionActiveID]: {
          ...(checkActionProg[checkActionActiveID] || { stage: 'progress', msg: '' }),
          percent: p.percent || 0,
          speed_mb: p.speed_mb || 0,
        } }
      }
    })
    EventsOn('autoreseed_ddl:status', p => {
      autoReseedStatus = p.msg || p.stage || ''
      if (p.msg) addLog('AUTO', p.msg)
    })
    EventsOn('autoreseed_ddl:progress', p => {
      autoReseedDDLProgress = {
        percent: p.percent || 0,
        speed_mb: p.speed_mb || 0,
        bytes: p.bytes || 0,
        total: p.total || 0,
      }
    })
    EventsOn('api:log', entry => {
      apiLogs = [entry, ...apiLogs].slice(0, API_LOG_MAX)
    })
    // Events du workflow complet (DDL→FTP+torrent+recheck) — on les route vers
    // la demande active (reqActiveID) pour afficher barre + vitesse dans le UI.
    EventsOn('autoreseed_full:status', p => {
      if (reqActiveID) {
        reqProgress = { ...reqProgress, [reqActiveID]: { ...(reqProgress[reqActiveID] || {}), stage: p.stage || '', msg: p.msg || '' } }
      }
      if (checkActionActiveID) {
        checkActionProg = { ...checkActionProg, [checkActionActiveID]: {
          ...(checkActionProg[checkActionActiveID] || {}),
          stage: p.stage || 'progress',
          msg: p.msg || '',
        } }
      }
      if (p.msg) addLog('AUTO', p.msg)
    })
    EventsOn('autoreseed_full:progress', p => {
      if (reqActiveID) {
        reqProgress = { ...reqProgress, [reqActiveID]: {
          ...(reqProgress[reqActiveID] || {}),
          stage: 'ftp',
          percent: p.percent || 0,
          speed_mb: p.speed_mb || 0,
          bytes: p.bytes || 0,
          total: p.total || 0,
        } }
      }
      if (checkActionActiveID) {
        checkActionProg = { ...checkActionProg, [checkActionActiveID]: {
          ...(checkActionProg[checkActionActiveID] || { stage: 'ftp', msg: '' }),
          percent: p.percent || 0,
          speed_mb: p.speed_mb || 0,
        } }
      }
    })
    EventsOn('autoreseed_full:seedbox', p => {
      if (reqActiveID) {
        reqProgress = { ...reqProgress, [reqActiveID]: {
          ...(reqProgress[reqActiveID] || {}),
          stage: 'seedbox',
          msg: `Seedbox : ${(p.percent || 0).toFixed(0)}%`,
        } }
      }
      if (checkActionActiveID) {
        checkActionProg = { ...checkActionProg, [checkActionActiveID]: {
          ...(checkActionProg[checkActionActiveID] || {}),
          stage: 'seedbox',
          percent: p.percent || 0,
          speed_mb: p.speed_mb || 0,
          msg: `Seedbox : ${(p.percent || 0).toFixed(0)}%`,
        } }
      }
    })
    EventsOn('reseed:progress', p => {
      reseedPct = p.percent ?? 0
      reseedSpeed = p.speed_mb ?? 0
    })
  })

  onDestroy(() => EventsOff('nzb:status', 'nzb:parpar', 'nzb:nyuu', 'nzb:result', 'ddl:log'))

  async function saveConfig() {
    await SaveConfig(cfg)
    cfgSaved = true
    setTimeout(() => cfgSaved = false, 2000)
  }

  // --- Tests ---
  let testResults = {}
  let testLoading = {}

  async function runTest(key, fn) {
    testLoading[key] = true
    testLoading = testLoading
    try {
      testResults[key] = await fn()
    } catch(e) {
      testResults[key] = { ok: false, message: e.toString() }
    }
    testLoading[key] = false
    testLoading = testLoading
    testResults = testResults
  }
</script>

<div class="layout">
  <!-- Sidebar -->
  <aside class="sidebar" class:collapsed={sidebarCollapsed}>
    <button class="sidebar-toggle" on:click={() => sidebarCollapsed = !sidebarCollapsed} title={sidebarCollapsed ? 'Déplier la barre' : 'Replier la barre'}>
      {sidebarCollapsed ? '›' : '‹'}
    </button>
    <div class="brand">
      <img src={logo} alt="" class="brand-logo" />
      {#if !sidebarCollapsed}
        <div class="logo">GO Post Tools</div>
        {#if appVersion}<div class="brand-version">v{appVersion}</div>{/if}
        <div class="brand-author">By GANDALF</div>
      {/if}
    </div>
    <nav>
      {#each TABS as tab}
        <button class="nav-item" class:active={activeTab === tab.id}
          on:click={() => activeTab = tab.id} title={sidebarCollapsed ? tab.label : ''}>
          {sidebarCollapsed ? tab.label.split(' ')[0] : tab.label}
        </button>
      {/each}
    </nav>
    <div class="sidebar-footer">
      {#if updateInfo?.available}
        <button class="btn-update" on:click={() => showUpdateModal = true} title="Télécharger la nouvelle version">
          {sidebarCollapsed ? '🆕' : `🆕 Mise à jour v${updateInfo.latest}`}
        </button>
      {:else}
        <button class="btn-check-update" on:click={recheckUpdate} disabled={updateChecking} title="Vérifier les mises à jour">
          {#if updateChecking}⌛{:else}🔄{/if}
          {#if !sidebarCollapsed}<span>{updateCheckMsg || 'Vérifier maj'}</span>{/if}
        </button>
      {/if}
    </div>
  </aside>

  <!-- Main content -->
  <main class="content">

    <!-- HydrackerTab toujours monté pour préserver l'état -->
    <div style="display:{activeTab === 'hydracker' ? 'contents' : 'none'}">
      <HydrackerTab />
    </div>

    {#if activeTab !== 'hydracker'}

    <!-- ===== FICHES ===== -->
    {#if activeTab === 'fiches'}
      <div class="tab-content">
        <h2>🎞 Fiches Hydracker</h2>

        {#if !fichesSelected}
          <!-- Mode recherche -->
          <div class="section">
            <div class="section-header"><span>Recherche</span></div>
            <div class="field" style="display:flex;gap:6px;margin-bottom:10px">
              <button class="btn-test" class:active-chip={fichesMode === 'name'} on:click={() => fichesMode = 'name'}>🔤 Par nom</button>
              <button class="btn-test" class:active-chip={fichesMode === 'hydracker_id'} on:click={() => fichesMode = 'hydracker_id'}>🆔 ID Hydracker</button>
              <button class="btn-test" class:active-chip={fichesMode === 'tmdb_id'} on:click={() => fichesMode = 'tmdb_id'}>🎬 ID TMDB</button>
            </div>
            <div class="field">
              <div class="pwd-row">
                <input type="text" bind:value={fichesQuery}
                  placeholder={fichesMode === 'name' ? 'Titre du film/série…' : fichesMode === 'hydracker_id' ? 'Ex: 62544' : 'Ex: 550 (Fight Club)'}
                  on:keydown={e => e.key === 'Enter' && !fichesLoading && fichesSearch()} />
                <button class="btn-save" on:click={fichesSearch} disabled={fichesLoading || !fichesQuery}>
                  {fichesLoading ? '…' : '🔍 Chercher'}
                </button>
              </div>
            </div>
            {#if fichesError}
              <div style="color:#ff9585;font-size:12px;margin-top:6px">⚠ {fichesError}</div>
            {/if}
          </div>

          <!-- Résultats en grille -->
          {#if fichesResults.length}
            <div class="section">
              <div class="section-header"><span>Résultats ({fichesResults.length})</span></div>
              <div class="fiches-grid">
                {#each fichesResults as f}
                  <button class="fiche-card" on:click={() => fichesOpen(f)} title={f.name}>
                    {#if f.poster}
                      <img src={f.poster} alt={f.name} loading="lazy" />
                    {:else}
                      <div class="fiche-no-poster">📽</div>
                    {/if}
                    <div class="fiche-info">
                      <div class="fiche-name">{f.name}</div>
                      <div class="fiche-meta">
                        {f.type === 'series' ? '📺 Série' : '🎬 Film'}
                        {#if f.release_date}· {f.release_date.slice(0,4)}{/if}
                        {#if f.score}· ⭐ {f.score.toFixed(1)}{/if}
                      </div>
                      <div class="fiche-id">#{f.id}</div>
                    </div>
                  </button>
                {/each}
              </div>
            </div>
          {/if}
        {:else}
          <!-- Détail d'une fiche -->
          <div class="section">
            <div style="display:flex;gap:16px;align-items:flex-start">
              <button class="btn-test" on:click={fichesBackToResults}>← Retour</button>
              {#if fichesSelected.poster}
                <img src={fichesSelected.poster} alt={fichesSelected.name} style="width:120px;border-radius:8px" />
              {/if}
              <div style="flex:1">
                <h3 style="margin:0 0 6px">{fichesSelected.name}</h3>
                <div style="color:var(--text3);font-size:12px;margin-bottom:8px">
                  {fichesSelected.type === 'series' ? '📺 Série' : '🎬 Film'}
                  {#if fichesSelected.release_date}· {fichesSelected.release_date.slice(0,4)}{/if}
                  · Hydracker #{fichesSelected.id}
                  {#if fichesSelected.imdb_id}· IMDb {fichesSelected.imdb_id}{/if}
                </div>
                <!-- Notes (TMDB depuis Hydracker + IMDb depuis proxytmdb) -->
                <div style="display:flex;gap:14px;margin-bottom:10px;font-size:12px;color:var(--text2)">
                  {#if fichesSelected.score || fichesSelected.rating}
                    <span>⭐ <b>TMDB</b> {(fichesSelected.score || fichesSelected.rating).toFixed(1)}</span>
                  {/if}
                  {#if fichesImdbInfo?.note_imdb}
                    <span style="color:#ffd60a">🟡 <b>IMDb</b> {fichesImdbInfo.note_imdb} <span style="opacity:0.7">({fichesImdbInfo.vote_imdb?.toLocaleString() || ''} votes)</span></span>
                  {/if}
                </div>
                <button class="btn-test" on:click={() => OpenBrowser(`https://hydracker.com/titles/${fichesSelected.id}`)}>
                  🌐 Ouvrir sur Hydracker
                </button>
                {#if fichesSelected.imdb_id}
                  <button class="btn-test" on:click={() => OpenBrowser(`https://www.imdb.com/title/${fichesSelected.imdb_id}`)}>🟡 IMDb</button>
                {/if}
                {#if fichesSelected.tmdb_id}
                  <button class="btn-test" on:click={() => OpenBrowser(`https://www.themoviedb.org/${fichesSelected.type === 'series' ? 'tv' : 'movie'}/${fichesSelected.tmdb_id}`)}>🎬 TMDB</button>
                {/if}
              </div>
            </div>

            <!-- Watch providers FR (Netflix, Disney+, etc.) -->
            {#if fichesProviders && fichesProviders.FR}
              {@const fr = fichesProviders.FR}
              <div style="margin-top:14px;padding:10px;background:rgba(255,255,255,0.025);border-radius:8px">
                <div style="font-size:11px;color:var(--text3);margin-bottom:6px;text-transform:uppercase;letter-spacing:0.3px">📺 Streaming FR</div>
                <div style="display:flex;flex-wrap:wrap;gap:8px;align-items:center">
                  {#each (fr.flatrate || []) as p}
                    <div style="display:flex;align-items:center;gap:5px;padding:4px 8px;background:rgba(126,240,192,0.08);border:1px solid rgba(126,240,192,0.25);border-radius:6px;font-size:11px">
                      <img src={`https://image.tmdb.org/t/p/w45${p.logo_path}`} alt={p.provider_name} style="width:18px;height:18px;border-radius:3px" />
                      <span>{p.provider_name}</span>
                    </div>
                  {/each}
                  {#each (fr.rent || []) as p}
                    <div style="display:flex;align-items:center;gap:5px;padding:4px 8px;background:rgba(255,255,255,0.04);border:1px solid var(--border);border-radius:6px;font-size:11px;opacity:0.85" title="Location">
                      <img src={`https://image.tmdb.org/t/p/w45${p.logo_path}`} alt={p.provider_name} style="width:18px;height:18px;border-radius:3px" />
                      <span>{p.provider_name} <span style="color:var(--text3)">(loc)</span></span>
                    </div>
                  {/each}
                  {#if !(fr.flatrate?.length) && !(fr.rent?.length)}
                    <span style="color:var(--text3);font-size:11px">Pas dispo en streaming en France actuellement.</span>
                  {/if}
                </div>
              </div>
            {:else if fichesProvidersLoading}
              <div style="margin-top:10px;color:var(--text3);font-size:11px">⏳ Chargement des providers streaming…</div>
            {/if}
          </div>

          <div class="section">
            <div class="section-header"><span>Contenu</span></div>
            {#if fichesContentLoading}
              <div style="color:var(--text3);font-size:12px">Chargement…</div>
            {:else if fichesContent}
              <!-- Tabs : Torrents / NZB / Liens DDL -->
              <div class="field" style="display:flex;gap:6px;margin-bottom:12px">
                <button class="btn-test" class:active-chip={fichesContentTab === 'torrents'} on:click={() => fichesContentTab = 'torrents'}>
                  📦 Torrents ({fichesContent.torrents?.torrents?.length || 0})
                </button>
                <button class="btn-test" class:active-chip={fichesContentTab === 'nzbs'} on:click={() => fichesContentTab = 'nzbs'}>
                  📰 NZB ({fichesContent.nzbs?.nzbs?.length || 0})
                </button>
                <button class="btn-test" class:active-chip={fichesContentTab === 'liens'} on:click={() => fichesContentTab = 'liens'}>
                  🔗 Liens DDL ({fichesContent.liens?.liens?.length || 0})
                </button>
              </div>

              {#if fichesContentTab === 'torrents'}
                {#if fichesContent.torrents?.torrents?.length}
                  <div class="content-grid">
                    {#each fichesContent.torrents.torrents as t}
                      <div class="content-card">
                        <div class="cc-body">
                          <div class="cc-head">
                            <span class="cc-id">#{t.id}</span>
                            {#if t.qual?.qual}<span class="cc-chip cc-chip-qual">{t.qual.qual}</span>{/if}
                            {#if t.saison || t.episode}<span class="cc-chip cc-chip-se">S{String(t.saison||0).padStart(2,'0')}E{String(t.episode||0).padStart(2,'0')}</span>{/if}
                            {#if t.full_saison}<span class="cc-chip cc-chip-se">Saison complète</span>{/if}
                            {#if t.size || t.taille}<span class="cc-chip cc-chip-size">{fmtSize(t.size || t.taille)}</span>{/if}
                            {#if (t.seeders ?? null) !== null}
                              <span class="cc-chip cc-chip-seed" style="color:{(t.seeders||0)===0?'#ff6b6b':((t.seeders||0)<=2?'#ffd60a':'#7ef0c0')}">🌱 {t.seeders||0}</span>
                            {/if}
                          </div>
                          <div class="cc-name" title={t.torrent_name || t.name || ''}>{t.torrent_name || t.name || '(sans nom)'}</div>
                          <div class="cc-tags">
                            {#each (t.langues_compact || []) as la}<span class="cc-tag cc-tag-lang">{langFlag(la.name)} {la.name}</span>{/each}
                            {#each (t.subs_compact || []) as s}<span class="cc-tag cc-tag-sub">💬 {s.name}</span>{/each}
                            {#if t.author}<span class="cc-tag cc-tag-author">👤 {t.author}</span>{/if}
                          </div>
                        </div>
                        <div class="cc-actions">
                          <button class="btn-test btn-icon" title="Voir le NFO" on:click={() => openNfo('torrents', t.id, t.torrent_name || t.name || '')}>ⓘ</button>
                          <button class="btn-test" on:click={() => downloadLienFromFiche({id:t.id, download_url:t.download_url, name:(t.torrent_name || t.name || ('torrent-'+t.id)) + '.torrent'})}>⬇ Torrent</button>
                        </div>
                      </div>
                    {/each}
                  </div>
                {:else}
                  <div style="color:var(--text3);font-size:12px">Aucun torrent partagé via API pour cette fiche.</div>
                {/if}
              {:else if fichesContentTab === 'nzbs'}
                {#if fichesContent.nzbs?.nzbs?.length}
                  <div class="content-grid">
                    {#each fichesContent.nzbs.nzbs as n}
                      <div class="content-card">
                        <div class="cc-body">
                          <div class="cc-head">
                            <span class="cc-id">#{n.id}</span>
                            {#if n.qual?.qual}<span class="cc-chip cc-chip-qual">{n.qual.qual}</span>{/if}
                            {#if n.saison || n.episode}<span class="cc-chip cc-chip-se">S{String(n.saison||0).padStart(2,'0')}E{String(n.episode||0).padStart(2,'0')}</span>{/if}
                            {#if n.size || n.taille}<span class="cc-chip cc-chip-size">{fmtSize(n.size || n.taille)}</span>{/if}
                          </div>
                          <div class="cc-name" title={n.name || ''}>{n.name || '(sans nom)'}</div>
                          <div class="cc-tags">
                            {#each (n.langues_compact || []) as la}<span class="cc-tag cc-tag-lang">{langFlag(la.name)} {la.name}</span>{/each}
                            {#each (n.subs_compact || []) as s}<span class="cc-tag cc-tag-sub">💬 {s.name}</span>{/each}
                            {#if n.author || n.id_user}<span class="cc-tag cc-tag-author">👤 {n.author || n.id_user}</span>{/if}
                          </div>
                        </div>
                        <div class="cc-actions">
                          <button class="btn-test btn-icon" title="Voir le NFO" on:click={() => openNfo('nzbs', n.id, n.name || '')}>ⓘ</button>
                          <button class="btn-test" on:click={() => downloadLienFromFiche({id:n.id, download_url:n.download_url, name:(n.name || ('nzb-'+n.id)) + '.nzb'})}>⬇ NZB</button>
                        </div>
                      </div>
                    {/each}
                  </div>
                {:else}
                  <div style="color:var(--text3);font-size:12px">Aucun NZB partagé via API pour cette fiche.</div>
                {/if}
              {:else if fichesContentTab === 'liens'}
                {#if fichesContent.liens?.liens?.length}
                  <div class="content-grid">
                    {#each fichesContent.liens.liens as l}
                      <div class="content-card">
                        <div class="cc-body">
                          <div class="cc-head">
                            <span class="cc-id">#{l.id}</span>
                            {#if l.qual?.qual}<span class="cc-chip cc-chip-qual">{l.qual.qual}</span>{/if}
                            {#if l.saison || l.episode}<span class="cc-chip cc-chip-se">S{String(l.saison||0).padStart(2,'0')}E{String(l.episode||0).padStart(2,'0')}</span>{/if}
                            {#if l.full_saison}<span class="cc-chip cc-chip-se">Saison complète</span>{/if}
                            {#if l.taille}<span class="cc-chip cc-chip-size">{fmtSize(l.taille)}</span>{/if}
                            {#if l.host?.name || l.id_host}<span class="cc-chip cc-chip-host">{l.host?.name || ('host#'+l.id_host)}</span>{/if}
                          </div>
                          {#if ddlFilenames[l.id]?.state === 'ok'}
                            <div class="cc-name" title={ddlFilenames[l.id].filename}>{ddlFilenames[l.id].filename}</div>
                            <div class="cc-name cc-name-mono cc-sub-url" title={l.lien || ''}>{l.lien}</div>
                          {:else if ddlFilenames[l.id]?.state === 'loading'}
                            <div class="cc-name cc-name-mono cc-sub-url">{l.lien}</div>
                            <div class="cc-name-loading">⏳ Résolution du nom du fichier…</div>
                          {:else if ddlFilenames[l.id]?.state === 'err'}
                            <div class="cc-name cc-name-mono cc-sub-url">{l.lien}</div>
                            <div class="cc-name-loading" style="color:#ff9585" title={ddlFilenames[l.id].error}>⚠ {ddlFilenames[l.id].error.length > 80 ? ddlFilenames[l.id].error.slice(0,80)+'…' : ddlFilenames[l.id].error}</div>
                          {:else}
                            <div class="cc-name cc-name-mono" title={l.lien || ''}>{l.lien || '(URL masquée — clic Ouvrir pour résoudre)'}</div>
                          {/if}
                          <div class="cc-tags">
                            {#each (l.langues_compact || []) as la}<span class="cc-tag cc-tag-lang">{langFlag(la.name)} {la.name}</span>{/each}
                            {#each (l.subs_compact || []) as s}<span class="cc-tag cc-tag-sub">💬 {s.name}</span>{/each}
                            {#if l.id_user}<span class="cc-tag cc-tag-author">👤 {l.id_user}</span>{/if}
                          </div>
                        </div>
                        <div class="cc-actions">
                          <button class="btn-test btn-icon" title="Voir le NFO" on:click={() => openNfo('liens', l.id, l.lien || '')}>ⓘ</button>
                          <button class="btn-test" on:click={() => OpenBrowser(l.lien)} disabled={!l.lien}>🌐 Ouvrir</button>
                        </div>
                      </div>
                    {/each}
                  </div>
                {:else}
                  <div style="color:var(--text3);font-size:12px">Aucun DDL partagé via API pour cette fiche.</div>
                {/if}
              {/if}

              {#if fichesContent.charged > 0}
                <div style="margin-top:10px;color:var(--text3);font-size:11px">💰 Charged: {fichesContent.charged.toFixed(3)}€</div>
              {/if}
            {:else}
              <div style="color:#ff9585;font-size:12px">Erreur lors du chargement du contenu</div>
            {/if}
          </div>
        {/if}
      </div>

    <!-- ===== DEMANDES DE RESEED ===== -->
    {:else if activeTab === 'requests'}
      <div class="tab-content">
        <h2>📋 Demandes de reseed</h2>
        <div class="section">
          <div class="section-header"><span>Filtres</span></div>
          <div class="field" style="display:flex;gap:6px;flex-wrap:wrap">
            <button class="btn-test" class:active-chip={reqFilter === 'pending'} on:click={() => { reqFilter = 'pending'; loadReseedRequests() }}>⏳ Pending</button>
            <button class="btn-test" class:active-chip={reqFilter === 'mine'} on:click={() => { reqFilter = 'mine'; loadReseedRequests() }}>👤 Les miennes</button>
            <button class="btn-test" class:active-chip={reqFilter === 'done'} on:click={() => { reqFilter = 'done'; loadReseedRequests() }}>✓ Done</button>
            <button class="btn-test" class:active-chip={reqFilter === 'rejected'} on:click={() => { reqFilter = 'rejected'; loadReseedRequests() }}>✗ Rejected</button>
            <button class="btn-test" class:active-chip={reqFilter === 'all'} on:click={() => { reqFilter = 'all'; loadReseedRequests() }}>📋 Toutes</button>
            <button class="btn-test" on:click={loadReseedRequests} disabled={reqLoading}>🔄 Rafraîchir</button>
            {#if reqFilter === 'pending' && reqList.length > 0}
              <button class="btn-save" on:click={processAllPending} disabled={reqLoading || Object.values(reqProcessing).some(v => v)}>
                ⚡ Tout traiter ({reqList.length})
              </button>
            {/if}
          </div>
        </div>

        <div class="section">
          <div class="section-header">
            <span>Demandes ({reqList.length})</span>
            {#if reqTotalPages > 1}
              <span style="color:var(--text3);font-size:11px">
                Page {reqPage}/{reqTotalPages}
                <button class="btn-test" style="margin-left:6px" on:click={() => { if (reqPage > 1) { reqPage--; loadReseedRequests() } }} disabled={reqPage <= 1}>‹</button>
                <button class="btn-test" on:click={() => { if (reqPage < reqTotalPages) { reqPage++; loadReseedRequests() } }} disabled={reqPage >= reqTotalPages}>›</button>
              </span>
            {/if}
          </div>

          {#if reqLoading && reqList.length === 0}
            <div style="color:var(--text3);font-size:12px">Chargement…</div>
          {:else if reqError}
            <div style="color:#ff9585;font-size:12px">⚠ {reqError}</div>
          {:else if reqList.length === 0}
            <div style="color:var(--text3);font-size:12px">Aucune demande dans ce filtre.</div>
          {:else}
            {#each reqList as req}
              {@const prog = reqProgress[req.id]}
              <div class="req-card">
                <div style="display:flex;gap:12px;align-items:flex-start">
                  {#if req.torrent?.title?.poster}
                    <img src={req.torrent.title.poster} alt="" style="width:56px;height:84px;object-fit:cover;border-radius:6px" loading="lazy" />
                  {/if}
                  <div style="flex:1;min-width:0">
                    <div style="font-weight:600;font-size:13px;color:var(--text);margin-bottom:4px">
                      {req.torrent?.title?.name || '(titre inconnu)'}
                      <span style="color:var(--text3);font-size:11px;font-weight:normal">· fiche #{req.torrent?.title_id}</span>
                    </div>
                    <div style="font-size:11px;color:var(--text3);word-break:break-all;margin-bottom:4px">{req.torrent?.torrent_name}</div>
                    <div style="display:flex;gap:12px;font-size:11px;color:var(--text3);flex-wrap:wrap">
                      <span>🌱 Seeders: <b style={req.torrent?.seeders === 0 ? 'color:#ff9585' : 'color:#7ef0c0'}>{req.torrent?.seeders ?? '?'}</b></span>
                      <span>👤 Demande: <b>{req.requester?.username || '#' + req.requester_id}</b></span>
                      <span>📤 Uploader: <b>{req.uploader?.username || '#' + req.uploader_id}</b></span>
                      <span>📅 {req.created_at?.slice(0,10)}</span>
                      <span class="req-status req-status-{req.status}">{req.status}</span>
                    </div>
                  </div>
                  <div style="display:flex;flex-direction:column;gap:6px;align-self:center">
                    <button class="btn-save" on:click={() => processReseedRequest(req)} disabled={reqProcessing[req.id] || req.status !== 'pending'} title="Reseed complet : DDL → FTP + .torrent → seedbox + force recheck">
                      {reqProcessing[req.id] ? '…' : '⚡ Traiter'}
                    </button>
                    <button class="btn-test" on:click={() => OpenBrowser(`https://hydracker.com/titles/${req.torrent?.title_id}`)}>🌐 Fiche</button>
                  </div>
                </div>
                {#if prog && (reqProcessing[req.id] || prog.stage === 'done' || prog.stage === 'error')}
                  <div class="req-progress">
                    <div style="display:flex;justify-content:space-between;font-size:11px;color:var(--text3);margin-bottom:4px">
                      <span style="color:{prog.stage === 'error' ? '#ff6b6b' : (prog.stage === 'done' ? '#7ef0c0' : 'var(--text2)')}">
                        {prog.stage === 'ftp' ? '⬆ FTP' : prog.stage === 'seedbox' ? '📦 Seedbox' : prog.stage === 'torrent_dl' ? '⬇ .torrent' : prog.stage === 'parsed' ? '✓ .torrent parsé' : prog.stage === 'ddl_search' ? '🔍 DDL' : prog.stage === 'ddl_picked' ? '🎯 DDL choisi' : prog.stage === 'token' ? '🔑 1fichier' : prog.stage === 'download' ? '⬇ DDL' : prog.stage === 'ftp_done' ? '✓ FTP' : prog.stage === 'recheck' ? '♻ Recheck' : prog.stage === 'done' ? '✓ Terminé' : prog.stage === 'error' ? '✗ Erreur' : prog.stage}
                        {#if prog.msg} — {prog.msg}{/if}
                      </span>
                      {#if prog.stage === 'ftp' && prog.total > 0}
                        <span style="font-family:monospace">
                          {prog.percent.toFixed(1)}% · {prog.speed_mb.toFixed(1)} MB/s · {(prog.bytes/1e9).toFixed(2)}/{(prog.total/1e9).toFixed(2)} GB
                        </span>
                      {/if}
                    </div>
                    {#if prog.stage !== 'error'}
                      <div class="progress-bar">
                        <div class="progress-fill" class:done={prog.stage === 'done'} style="width:{prog.percent || 0}%"></div>
                      </div>
                    {/if}
                  </div>
                {/if}
              </div>
            {/each}
          {/if}
        </div>
      </div>

    <!-- ===== MES UPLOADS ===== -->
    {:else if activeTab === 'myuploads'}
      <div class="tab-content">
        <h2>📤 Mes uploads</h2>
        <div class="section">
          <div class="section-header">
            <span>Type</span>
            <span style="color:var(--text3);font-size:11px">user: <b>{myUsername}</b></span>
          </div>
          <div class="field" style="display:flex;gap:6px;flex-wrap:wrap">
            <button class="btn-test" class:active-chip={myTab === 'torrents'} on:click={() => { myTab = 'torrents'; myPage = 1; loadMyUploads() }}>📦 Torrents</button>
            <button class="btn-test" class:active-chip={myTab === 'liens'} on:click={() => { myTab = 'liens'; myPage = 1; loadMyUploads() }}>🔗 DDL</button>
            <button class="btn-test" on:click={loadMyUploads} disabled={myLoading}>🔄 Rafraîchir</button>
          </div>
        </div>

        <div class="section">
          <div class="section-header">
            <span>Items ({myItems.length})</span>
            {#if myTotalPages > 1}
              <span style="color:var(--text3);font-size:11px">
                Page {myPage}/{myTotalPages}
                <button class="btn-test" style="margin-left:6px" on:click={() => { if (myPage > 1) { myPage--; loadMyUploads() } }} disabled={myPage <= 1}>‹</button>
                <button class="btn-test" on:click={() => { if (myPage < myTotalPages) { myPage++; loadMyUploads() } }} disabled={myPage >= myTotalPages}>›</button>
              </span>
            {/if}
          </div>
          {#if myLoading && myItems.length === 0}
            <div style="color:var(--text3);font-size:12px">Chargement…</div>
          {:else if myError}
            <div style="color:#ff9585;font-size:12px">⚠ {myError}</div>
          {:else if myItems.length === 0}
            <div style="color:var(--text3);font-size:12px">Aucun item.</div>
          {:else}
            {#each myItems as item}
              <div class="my-item">
                <div style="flex:1;min-width:0">
                  <div style="font-weight:600;font-size:12px;color:var(--text);margin-bottom:3px">
                    #{item.id}
                    · {item.name || item.torrent_name || (myTab === 'liens' ? item.lien : '(sans nom)')}
                  </div>
                  <div style="display:flex;gap:10px;flex-wrap:wrap;font-size:11px;color:var(--text3)">
                    <span>fiche #{item.title_id}</span>
                    {#if item.qualite || item.quality}<span>qual: {qualityName(item.qualite || item.quality)}</span>{/if}
                    {#if item.saison || item.episode}<span>S{String(item.saison||0).padStart(2,'0')}E{String(item.episode||0).padStart(2,'0')}</span>{/if}
                    {#if item.taille || item.size}<span>{((item.taille||item.size)/1e9).toFixed(2)} GB</span>{/if}
                    {#if item.host?.name}<span>host: {item.host.name}</span>{/if}
                    {#if item.active === 0}<span style="color:#ff9585">⚠ inactif</span>{/if}
                    <span>📅 {(item.created_at || '').slice(0,10)}</span>
                  </div>
                </div>
                <div style="display:flex;flex-direction:column;gap:4px;align-self:center">
                  <button class="btn-test" on:click={() => startEdit(item)} title="Modifier">✏ Modifier</button>
                  <button class="btn-test" style="color:#ff6b6b;border-color:rgba(255,107,107,0.35)" on:click={() => startDelete(item)} title="Supprimer">🗑 Supprimer</button>
                </div>
              </div>
            {/each}
          {/if}
        </div>

        <!-- Modal Édition -->
        {#if editingItem}
          <div class="modal-backdrop" on:click|self={cancelEdit}>
            <div class="modal-card">
              <div class="modal-title">✏ Modifier #{editingItem.id}</div>
              <div class="modal-hint">Laisse à 0 les champs que tu ne veux pas changer.</div>
              <div class="field">
                <label>Qualité (ID)</label>
                <select bind:value={editPayload.quality}>
                  <option value={0}>— Inchangé —</option>
                  {#each qualityOptions as q}
                    <option value={q.id}>{q.name} (#{q.id})</option>
                  {/each}
                </select>
              </div>
              <div class="field" style="display:flex;gap:10px">
                <div style="flex:1">
                  <label>Saison</label>
                  <input type="number" min="0" bind:value={editPayload.saison} />
                </div>
                <div style="flex:1">
                  <label>Épisode</label>
                  <input type="number" min="0" bind:value={editPayload.episode} />
                </div>
              </div>
              <div class="field">
                <label>Langue (ID)</label>
                <input type="number" min="0" bind:value={editPayload.lang} placeholder="0 = inchangé · ex: 8 = TrueFrench" />
              </div>
              <div class="field">
                <label>État</label>
                <div style="display:flex;gap:6px">
                  <button class="btn-test" class:active-chip={editPayload.active === -1} on:click={() => editPayload.active = -1}>Inchangé</button>
                  <button class="btn-test" class:active-chip={editPayload.active === 1} on:click={() => editPayload.active = 1}>✓ Actif</button>
                  <button class="btn-test" class:active-chip={editPayload.active === 0} on:click={() => editPayload.active = 0}>⚠ Inactif</button>
                </div>
              </div>
              <div class="post-actions">
                <button class="btn-save" on:click={saveEdit}>💾 Enregistrer</button>
                <button class="btn-test" on:click={cancelEdit}>Annuler</button>
              </div>
            </div>
          </div>
        {/if}

        <!-- Modal Delete -->
        {#if deletingItem}
          <div class="modal-backdrop" on:click|self={cancelDelete}>
            <div class="modal-card" style="border-color:rgba(255,107,107,0.55)">
              <div class="modal-title" style="color:#ff6b6b">🗑 Suppression définitive</div>
              <div class="modal-hint">
                Tu es sur le point de supprimer <b>définitivement</b> cet item :
              </div>
              <div style="background:rgba(0,0,0,0.25);padding:10px;border-radius:8px;margin:10px 0;font-size:12px">
                <div><b>#{deletingItem.id}</b> · {deletingItem.name || deletingItem.torrent_name || deletingItem.lien || ''}</div>
                <div style="color:var(--text3);font-size:11px;margin-top:3px">fiche #{deletingItem.title_id}{deletingItem.saison || deletingItem.episode ? ` · S${String(deletingItem.saison||0).padStart(2,'0')}E${String(deletingItem.episode||0).padStart(2,'0')}` : ''}</div>
              </div>
              <div class="field">
                <label>Tape l'ID <b>{deletingItem.id}</b> pour confirmer :</label>
                <input type="text" bind:value={deleteConfirmInput} placeholder={deletingItem.id} />
              </div>
              <div class="post-actions">
                <button class="btn-save" style="background:#ff6b6b;border-color:#ff6b6b" on:click={confirmDelete} disabled={deleteConfirmInput.trim() !== String(deletingItem.id)}>
                  🗑 Supprimer définitivement
                </button>
                <button class="btn-test" on:click={cancelDelete}>Annuler</button>
              </div>
            </div>
          </div>
        {/if}
      </div>

    <!-- ===== LOG API ===== -->
    {:else if activeTab === 'apilog'}
      <div class="tab-content">
        <h2>🔬 Log API</h2>
        <div class="section">
          <div class="section-header">
            <span>Requêtes capturées ({filteredApiLogs.length}/{apiLogs.length})</span>
            <button class="btn-test" on:click={clearApiLogs}>🗑 Vider</button>
          </div>
          <div class="field" style="display:flex;gap:6px;flex-wrap:wrap;align-items:center">
            <button class="btn-test" class:active-chip={apiLogFilter === 'all'} on:click={() => apiLogFilter = 'all'}>Toutes</button>
            <button class="btn-test" class:active-chip={apiLogFilter === 'ok'} on:click={() => apiLogFilter = 'ok'}>✓ OK (2xx)</button>
            <button class="btn-test" class:active-chip={apiLogFilter === 'error'} on:click={() => apiLogFilter = 'error'}>✗ Erreurs</button>
            <input type="text" bind:value={apiLogSearch} placeholder="Filtrer par URL / body / erreur…" style="flex:1;min-width:200px" />
          </div>
        </div>

        <div class="section">
          {#if filteredApiLogs.length === 0}
            <div style="color:var(--text3);font-size:12px">Aucune requête (fais une action dans l'app pour en générer).</div>
          {:else}
            {#each filteredApiLogs as e, i}
              <div class="api-log-row" class:api-log-err={e.error || e.status >= 400} on:click={() => apiLogExpanded = { ...apiLogExpanded, [i]: !apiLogExpanded[i] }}>
                <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap;font-size:11px">
                  <span style="color:var(--text3);font-family:monospace">{e.ts}</span>
                  <span class="api-log-method api-log-method-{e.method}">{e.method}</span>
                  <span style="color:{e.status >= 200 && e.status < 300 ? '#7ef0c0' : (e.status >= 400 ? '#ff6b6b' : 'var(--text3)')};font-weight:600;min-width:40px">{e.status || '—'}</span>
                  <span style="color:var(--text3);font-family:monospace">{e.duration_ms}ms</span>
                  <span style="flex:1;font-family:monospace;word-break:break-all;color:var(--text2)">{e.url}</span>
                </div>
                {#if e.error}
                  <div style="color:#ff6b6b;font-size:11px;margin-top:4px">⚠ {e.error}</div>
                {/if}
                {#if apiLogExpanded[i] && e.body_preview}
                  <pre style="background:rgba(0,0,0,0.3);padding:8px;border-radius:4px;margin-top:6px;font-size:10px;color:#7ef0c0;overflow-x:auto;white-space:pre-wrap;max-height:240px;overflow-y:auto">{e.body_preview}</pre>
                {/if}
              </div>
            {/each}
          {/if}
        </div>
      </div>

    <!-- ===== HISTORIQUE ===== -->
    {:else if activeTab === 'history'}
      <div class="tab-content">
        <h2>📚 Historique</h2>
        <div class="hist-stats">
          <span class="hist-stat">Total <b>{histStats.total}</b></span>
          <span class="hist-stat ok">OK <b>{histStats.ok}</b></span>
          <span class="hist-stat err">Erreurs <b>{histStats.error}</b></span>
          <span class="hist-stat">Torrent <b>{histStats.torrent}</b></span>
          <span class="hist-stat">NZB <b>{histStats.nzb}</b></span>
          <span class="hist-stat">DDL <b>{histStats.ddl}</b></span>
        </div>
        <div class="hist-filters">
          <input class="hist-search" type="text" placeholder="🔍 Recherche (titre, fichier, lien…)" bind:value={histQuery} on:input={loadHistory} />
          <div class="hist-type-btns">
            <button class:active={histFilter === ''} on:click={() => { histFilter = ''; loadHistory() }}>Tous</button>
            <button class:active={histFilter === 'torrent'} on:click={() => { histFilter = 'torrent'; loadHistory() }}>🧲 Torrent</button>
            <button class:active={histFilter === 'nzb'} on:click={() => { histFilter = 'nzb'; loadHistory() }}>📰 NZB</button>
            <button class:active={histFilter === 'ddl'} on:click={() => { histFilter = 'ddl'; loadHistory() }}>🔗 DDL</button>
          </div>
        </div>
        {#if histLoading}
          <div class="hist-empty">Chargement…</div>
        {:else if histEntries.length === 0}
          <div class="hist-empty">Aucune entrée. Les posts s'ajoutent automatiquement ici.</div>
        {:else}
          <div class="hist-list">
            {#each histEntries as e}
              <div class="hist-row" class:err={e.status === 'error'}>
                <div class="hist-col-date">{formatDate(e.timestamp)}</div>
                <div class="hist-col-type hist-type-{e.type}">{e.type.toUpperCase()}</div>
                <div class="hist-col-main">
                  <div class="hist-title">{e.title_name || `#${e.title_id}`}{#if e.saison || e.episode} · S{String(e.saison).padStart(2,'0')}E{String(e.episode).padStart(2,'0')}{/if}</div>
                  <div class="hist-sub">{e.qualite_name || ''} · {e.filename}</div>
                  {#if e.links}<div class="hist-links">{e.links}</div>{/if}
                  {#if e.error}<div class="hist-error">✗ {e.error}</div>{/if}
                </div>
                <div class="hist-col-id">#{e.hydracker_id || '—'}</div>
                <button class="hist-del" on:click={() => deleteHistEntry(e.id)} title="Supprimer">✕</button>
              </div>
            {/each}
          </div>
        {/if}
      </div>

    <!-- ===== RESEED ===== -->
    {:else if activeTab === 'reseed'}
      <div class="tab-content">
        <h2>♻️ Reseed</h2>

        <!-- Auto-reseed : workflow 1 clic depuis un ID / URL Hydracker.
             L'app liste les torrents dispos, choisit le meilleur (FR + qualité),
             télécharge le .torrent et l'envoie direct au ruTorrent. Aucun MKV
             ni FTP nécessaire — idéal pour traiter les demandes de reseed. -->
        <div class="section">
          <div class="section-header"><span>⚡ Auto-reseed depuis une fiche Hydracker</span></div>
          <div class="field">
            <label>URL ou ID fiche Hydracker</label>
            <input type="text" bind:value={autoReseedInput}
              placeholder="https://hydracker.com/titles/12345 ou 12345"
              on:keydown={e => e.key === 'Enter' && !autoReseedLoading && launchAutoReseed()} />
          </div>
          <div class="field" style="display:flex;gap:10px">
            <div style="flex:1">
              <label>Saison <span style="color:var(--text3);font-weight:normal;font-size:11px">(0 = film)</span></label>
              <input type="number" min="0" bind:value={autoReseedSaison} placeholder="0" />
            </div>
            <div style="flex:1">
              <label>Épisode <span style="color:var(--text3);font-weight:normal;font-size:11px">(0 = saison complète / film)</span></label>
              <input type="number" min="0" bind:value={autoReseedEpisode} placeholder="0" />
            </div>
          </div>
          <div class="post-actions" style="flex-wrap:wrap">
            <button class="btn-save" on:click={launchAutoReseed} disabled={autoReseedLoading || !autoReseedInput || !cfg.seedbox_url}
              title="Liste les torrents partagés via API → choisit le meilleur (FR + 1080p) → seedbox">
              {autoReseedLoading ? '…' : '⚡ Torrent → seedbox'}
            </button>
            <button class="btn-save" on:click={launchAutoReseedDDL} disabled={autoReseedLoading || !autoReseedInput || !cfg.ftp_host || !cfg.one_fichier_api_key}
              title="Aucun torrent partagé ? Télécharge depuis un DDL 1fichier et push direct sur le FTP configuré (streaming, sans passer par le disque)">
              {autoReseedLoading ? '…' : '📦 DDL → FTP'}
            </button>
            {#if !cfg.seedbox_url}
              <span style="color:var(--text3);font-size:11px;align-self:center">💡 Seedbox requis pour torrent</span>
            {/if}
            {#if !cfg.ftp_host || !cfg.one_fichier_api_key}
              <span style="color:var(--text3);font-size:11px;align-self:center">💡 FTP + clé API 1fichier requis pour DDL</span>
            {/if}
          </div>
          {#if autoReseedLoading && autoReseedDDLProgress.total > 0}
            <div class="recap-row" style="margin-top:8px">
              <span class="recap-key">Progression</span>
              <span class="recap-val">{autoReseedDDLProgress.percent.toFixed(1)}% · {autoReseedDDLProgress.speed_mb.toFixed(1)} MB/s · {(autoReseedDDLProgress.bytes/1e9).toFixed(2)} / {(autoReseedDDLProgress.total/1e9).toFixed(2)} GB</span>
            </div>
          {/if}
          {#if autoReseedLoading && autoReseedStatus}
            <div class="recap-row" style="margin-top:8px"><span class="recap-key">Statut</span><span class="recap-val">{autoReseedStatus}</span></div>
          {/if}
          {#if autoReseedError}
            <div class="recap-row" style="margin-top:8px"><span class="recap-key">Erreur</span><span class="recap-val" style="color:#ff6b6b">{autoReseedError}</span></div>
          {/if}
          {#if autoReseedResult}
            <div style="margin-top:12px;padding:10px;background:rgba(126,240,192,0.08);border:1px solid rgba(126,240,192,0.25);border-radius:8px">
              <div style="color:#7ef0c0;font-weight:600;margin-bottom:6px">✓ Torrent ajouté à la seedbox</div>
              <div class="recap-row"><span class="recap-key">Torrent</span><span class="recap-val">#{autoReseedResult.torrent_id} — {autoReseedResult.torrent_name}</span></div>
              {#if autoReseedResult.size_bytes}
                <div class="recap-row"><span class="recap-key">Taille</span><span class="recap-val">{(autoReseedResult.size_bytes/1e9).toFixed(2)} GB</span></div>
              {/if}
              {#if autoReseedResult.seedbox_path}
                <div class="recap-row"><span class="recap-key">Seedbox</span><span class="recap-val recap-file">{autoReseedResult.seedbox_path}</span></div>
              {/if}
            </div>
          {/if}
        </div>

        <div class="section">
          <div class="section-header"><span>Fichiers</span></div>
          <div class="field">
            <label>Fichier .torrent</label>
            <div class="pwd-row">
              <input type="text" value={reseedTorrentPath} readonly placeholder="Aucun fichier sélectionné" />
              <button class="btn-test" on:click={reseedPickTorrent}>Parcourir</button>
            </div>
          </div>
          <div class="field">
            <label>Fichier MKV <span style="color:var(--text3);font-weight:normal;font-size:11px">(optionnel — requis uniquement pour re-seed via seedbox)</span></label>
            <div class="pwd-row">
              <input type="text" value={reseedMkvPath} readonly placeholder="Optionnel si tu n'as pas de seedbox" />
              <button class="btn-test" on:click={reseedPickMkv}>Parcourir</button>
              {#if reseedMkvPath}<button class="btn-test" on:click={() => reseedMkvPath = ''} title="Retirer">✕</button>{/if}
            </div>
          </div>
          <div class="post-actions">
            <button class="btn-save" on:click={reseedAnalyze} disabled={!reseedTorrentPath || reseedPrepLoading}>
              {reseedPrepLoading ? '…' : '🔍 Analyser'}
            </button>
            <button class="btn-reset" on:click={reseedReset}>↺ Réinitialiser</button>
          </div>
        </div>

        {#if reseedPrep}
          <div class="section">
            <div class="section-header"><span>Résultat de l'analyse</span></div>
            <div class="recap-row"><span class="recap-key">Nom torrent</span><span class="recap-val">{reseedPrep.torrent_name}</span></div>
            <div class="recap-row"><span class="recap-key">Fichier principal</span><span class="recap-val">{reseedPrep.first_file_name}</span></div>
            <div class="recap-row"><span class="recap-key">Taille</span><span class="recap-val">{(reseedPrep.size / 1e9).toFixed(2)} GB</span></div>
            <div class="recap-row"><span class="recap-key">Info hash</span><span class="recap-val recap-file">{reseedPrep.info_hash}</span></div>
            {#if reseedPrep.search}
              <div class="recap-row"><span class="recap-key">TMDB</span><span class="recap-val recap-id">#{reseedPrep.search.tmdb_id} — {reseedPrep.search.title_fr || reseedPrep.search.title_vo} ({reseedPrep.search.year})</span></div>
            {:else}
              <div class="recap-row"><span class="recap-key">Recherche TMDB</span><span class="recap-val" style="color:#ff9585">Aucun match</span></div>
            {/if}
            {#if reseedPrep.hydracker_fiche}
              <div class="recap-row"><span class="recap-key">Fiche Hydracker</span><span class="recap-val" style="color:#7ef0c0">✓ {reseedPrep.hydracker_fiche.name} (#{reseedPrep.hydracker_fiche.id})</span></div>
              <div class="post-actions" style="gap:8px;flex-wrap:wrap">
                {#if reseedMkvPath && cfg.seedbox_url}
                  <button class="btn-save" on:click={reseedConfirm} disabled={reseedRunning} title="Upload MKV sur FTP + ajoute le .torrent à ruTorrent + force re-check">
                    {reseedRunning ? '…' : '🔄 Re-seed via seedbox'}
                  </button>
                {/if}
                <button class="btn-save" on:click={() => reseedOpenInHydracker(reseedPrep)} title="Ouvre l'onglet Hydracker avec le .torrent et la fiche pré-remplis">
                  🎬 Poster sur Hydracker
                </button>
                {#if !reseedMkvPath && cfg.seedbox_url}
                  <span style="color:var(--text3);font-size:11px;align-self:center">💡 Fournis un MKV pour activer le re-seed seedbox</span>
                {:else if !cfg.seedbox_url}
                  <span style="color:var(--text3);font-size:11px;align-self:center">💡 Configure une seedbox dans Réglages pour activer le re-seed</span>
                {/if}
                <span style="color:var(--text3);font-size:11px;align-self:center">💡 Pour lister/télécharger les sources, utilise l'onglet 🎞 Fiches</span>
              </div>
            {:else}
              <div class="recap-row"><span class="recap-key">Fiche Hydracker</span><span class="recap-val" style="color:#ff9585">✗ Pas de fiche correspondante</span></div>
            {/if}
          </div>
        {/if}

        {#if reseedStage}
          <div class="section">
            <div class="section-header"><span>Progression</span></div>
            <div class="nzb-live-status" class:done={reseedStage === 'done'}>{reseedMsg}</div>
            {#if reseedStage === 'ftp' || reseedStage === 'ftp_done'}
              <div class="nzb-live-step">
                <span>FTP</span>
                <div class="progress-bar"><div class="progress-fill" style="width:{reseedPct}%"></div></div>
                <span class="pct">{reseedPct.toFixed(0)}%</span>
              </div>
              <div class="nzb-live-meta">
                {#if reseedSpeed}<span>⚡ {reseedSpeed.toFixed(1)} MB/s</span>{/if}
              </div>
            {/if}
          </div>
        {/if}
      </div>

    <!-- ===== CHECK TORRENT ===== -->
    {:else if activeTab === 'check'}
      <div class="tab-content check-tab">
        <div class="check-header">
          <h2>🔍 Check Torrent</h2>
        </div>

        <!-- Section 1 : Mes torrents Hydracker (par seeders) -->
        <div class="section">
          <div class="section-header">
            <span>🌱 Mes torrents Hydracker — par seeders ASC ({filteredMySeeds.length}/{mySeedsTorrents.length})</span>
            <div style="display:flex;gap:6px;align-items:center">
              {#if mySeedsTotalPages > 1}
                <span style="color:var(--text3);font-size:11px">Page {mySeedsPage}/{mySeedsTotalPages}</span>
                <button class="btn-test" on:click={() => { if (mySeedsPage > 1) { mySeedsPage--; loadMySeeds() } }} disabled={mySeedsPage <= 1}>‹</button>
                <button class="btn-test" on:click={() => { if (mySeedsPage < mySeedsTotalPages) { mySeedsPage++; loadMySeeds() } }} disabled={mySeedsPage >= mySeedsTotalPages}>›</button>
              {/if}
              <button class="btn-test" on:click={loadMySeeds} disabled={mySeedsLoading}>🔄 Rafraîchir</button>
            </div>
          </div>
          <div class="field" style="display:flex;gap:6px;flex-wrap:wrap">
            <button class="btn-test" class:active-chip={onlyMine} on:click={() => onlyMine = !onlyMine}
              title="Si actif : ne montre que les torrents que tu as ENCORE sur ta seedbox / qBit (filtre par info_hash)">
              {onlyMine ? '✅' : '⬜'} Sur ma seedbox uniquement{seedboxHashesLoaded ? ` (${seedboxHashes.size})` : ''}
            </button>
            <button class="btn-test" class:active-chip={mySeedsFilter === 'all'} on:click={() => mySeedsFilter = 'all'}>Tous ({mySeedsTorrents.length})</button>
            <button class="btn-test" class:active-chip={mySeedsFilter === '0seed'} on:click={() => mySeedsFilter = '0seed'} style="color:#ff6b6b">🔴 0 seed ({mySeedsTorrents.filter(t => (t.seeders||0) === 0).length})</button>
            <button class="btn-test" class:active-chip={mySeedsFilter === 'low'} on:click={() => mySeedsFilter = 'low'} style="color:#ffd60a">🟠 1-2 seeds ({mySeedsTorrents.filter(t => (t.seeders||0) > 0 && (t.seeders||0) <= 2).length})</button>
            <button class="btn-test" class:active-chip={mySeedsFilter === 'ok'} on:click={() => mySeedsFilter = 'ok'} style="color:#7ef0c0">🟢 3+ seeds ({mySeedsTorrents.filter(t => (t.seeders||0) >= 3).length})</button>
            <button class="btn-test" on:click={checkLocalMkv} disabled={localMkvChecking} title="Vérifier qu'un MKV local correspond à une fiche Hydracker (et voir les sources DDL/torrents dispos)">
              {localMkvChecking ? '…' : '🔍 Parcourir un MKV local'}
            </button>
          </div>

          {#if mySeedsLoading && !mySeedsTorrents.length}
            <div style="color:var(--text3);font-size:12px;margin-top:10px">Chargement…</div>
          {:else if mySeedsError}
            <div style="color:#ff6b6b;font-size:12px;margin-top:10px">⚠ {mySeedsError}</div>
          {:else if filteredMySeeds.length === 0}
            <div style="color:var(--text3);font-size:12px;margin-top:10px">Aucun torrent dans ce filtre.</div>
          {:else}
            {#each filteredMySeeds as t}
              {@const seeders = t.seeders || 0}
              {@const seedColor = seeders === 0 ? '#ff6b6b' : (seeders <= 2 ? '#ffd60a' : '#7ef0c0')}
              <div class="my-item" style="border-left: 3px solid {seedColor}">
                <div style="flex:1;min-width:0">
                  <div style="font-size:12px;font-weight:600;color:var(--text);margin-bottom:3px">
                    #{t.id} · {t.torrent_name || t.name || '(sans nom)'}
                  </div>
                  <div style="display:flex;gap:10px;flex-wrap:wrap;font-size:11px;color:var(--text3)">
                    <span style="color:{seedColor};font-weight:600">🌱 {seeders} seed{seeders > 1 ? 's' : ''}</span>
                    {#if t.leechers}<span>⬇ {t.leechers} leech</span>{/if}
                    <span>fiche #{t.title_id}</span>
                    {#if t.qualite || t.quality}<span>qual #{t.qualite || t.quality}</span>{/if}
                    {#if t.saison || t.episode}<span>S{String(t.saison||0).padStart(2,'0')}E{String(t.episode||0).padStart(2,'0')}</span>{/if}
                    {#if t.taille || t.size}<span>{((t.taille||t.size)/1e9).toFixed(2)} GB</span>{/if}
                    <span>📅 {(t.created_at || '').slice(0,10)}</span>
                  </div>
                </div>
                <div style="display:flex;flex-direction:column;gap:4px;align-self:center">
                  <button class="btn-save" on:click={() => autoReseedFromCheck(t)} disabled={mySeedsActioning[t.id]} title="Push .torrent sur seedbox (nécessite que quelqu'un seed pour que BT retrouve le fichier)">
                    {mySeedsActioning[t.id] ? '…' : '⚡ Torrent → seedbox'}
                  </button>
                  {#if cfg.ftp_host && cfg.one_fichier_api_key}
                    <button class="btn-save" style="background:#7ef0c0;color:#000" on:click={() => fullReseedFromCheck(t)} disabled={mySeedsActioning[t.id]}
                      title="Reseed complet : DDL 1fichier → FTP (nom exact torrent) + .torrent seedbox + force recheck">
                      {mySeedsActioning[t.id] ? '…' : '⚡⚡ Reseed complet'}
                    </button>
                  {/if}
                  <button class="btn-test" on:click={() => OpenBrowser(`https://hydracker.com/titles/${t.title_id}`)}>🌐 Fiche</button>
                  <button class="btn-test" style="background:#5a1a1a;color:#ff6b6b;border-color:#ff6b6b" on:click={() => askDeleteTorrent(t)} disabled={mySeedsActioning[t.id]} title="Supprime le .torrent sur Hydracker ET le fichier sur ton FTP">
                    🗑 Suppr +FTP
                  </button>
                </div>
              </div>
              {#if checkActionProg[t.id]}
                {@const prog = checkActionProg[t.id]}
                <div class="check-prog" class:done={prog.stage === 'done'} class:err={prog.stage === 'error'}>
                  <div class="check-prog-bar"><div class="check-prog-fill" style="width:{prog.percent || 0}%"></div></div>
                  <div class="check-prog-meta">
                    <span class="check-prog-msg">{prog.msg || prog.stage || '…'}</span>
                    <span class="check-prog-stats">
                      {(prog.percent || 0).toFixed(0)}%
                      {#if prog.speed_mb > 0} · ⚡ {prog.speed_mb.toFixed(1)} MB/s{/if}
                    </span>
                  </div>
                </div>
              {/if}
            {/each}
          {/if}
        </div>

        <!-- Modal confirmation suppression torrent + FTP -->
        {#if deleteTorrentModal}
          <div class="modal-backdrop" on:click|self={closeDeleteTorrentModal}>
            <div class="modal-card" style="max-width:560px">
              <div class="modal-title" style="color:#ff6b6b">🗑 Suppression définitive</div>
              {#if !deleteTorrentModal.result}
                <div class="modal-hint">{deleteTorrentModal.torrent.torrent_name || deleteTorrentModal.torrent.name || '#' + deleteTorrentModal.torrent.id}</div>
                <div style="margin:14px 0;font-size:12px;line-height:1.5;color:var(--text2)">
                  Cette action va :<br>
                  1️⃣ Supprimer le torrent <b>#{deleteTorrentModal.torrent.id}</b> sur Hydracker (DELETE)<br>
                  2️⃣ Récupérer le .torrent + parser les fichiers<br>
                  3️⃣ Supprimer le(s) fichier(s) sur ton FTP perso (puis FTP mod en fallback)<br>
                  <br>
                  <span style="color:#ff9585">⚠ Irréversible.</span>
                </div>
                {#if deleteTorrentModal.error}
                  <div style="color:#ff6b6b;font-size:12px;margin:10px 0">⚠ {deleteTorrentModal.error}</div>
                {/if}
                <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:14px">
                  <button class="btn-test" on:click={closeDeleteTorrentModal} disabled={deleteTorrentModal.loading}>Annuler</button>
                  <button class="btn-save" style="background:#7a1c1c;color:#fff" on:click={confirmDeleteTorrent} disabled={deleteTorrentModal.loading}>
                    {deleteTorrentModal.loading ? '⏳ Suppression…' : '🗑 Confirmer la suppression'}
                  </button>
                </div>
              {:else}
                {@const r = deleteTorrentModal.result}
                <div style="margin:14px 0;font-size:12px;line-height:1.6">
                  <div style="color:{r.hydracker_ok ? '#7ef0c0' : '#ff6b6b'}">
                    {r.hydracker_ok ? '✓' : '✗'} Hydracker : {r.hydracker_ok ? 'torrent supprimé' : (r.hydracker_err || 'échec')}
                  </div>
                  <div style="color:{r.seedbox_ok ? '#7ef0c0' : '#ffd60a'};margin-top:6px">
                    {r.seedbox_ok ? '✓' : '⚠'} Seedbox : {r.seedbox_ok ? `torrent retiré (${r.used_seedbox})` : (r.seedbox_err || 'pas de seedbox configurée')}
                  </div>
                  <div style="color:{r.ftp_deleted?.length ? '#7ef0c0' : '#ffd60a'};margin-top:6px">
                    {r.ftp_deleted?.length ? '✓' : '⚠'} FTP : {r.ftp_deleted?.length || 0} fichier(s) supprimé(s)
                    {#if r.used_ftp}<span style="color:var(--text3)"> (via {r.used_ftp})</span>{/if}
                  </div>
                  {#if r.ftp_deleted?.length}
                    <ul style="font-size:11px;color:var(--text3);margin:6px 0 0 22px">
                      {#each r.ftp_deleted as f}<li>{f}</li>{/each}
                    </ul>
                  {/if}
                  {#if r.ftp_errors?.length && !r.ftp_deleted?.length}
                    <div style="font-size:11px;color:#ff9585;margin-top:6px">Erreurs FTP :</div>
                    <ul style="font-size:11px;color:#ff9585;margin:4px 0 0 22px">
                      {#each r.ftp_errors as e}<li>{e}</li>{/each}
                    </ul>
                  {/if}
                </div>
                <div style="display:flex;justify-content:flex-end;margin-top:12px">
                  <button class="btn-save" on:click={closeDeleteTorrentModal}>Fermer</button>
                </div>
              {/if}
            </div>
          </div>
        {/if}

        <!-- Modal résultat du check MKV local -->
        {#if localMkvCheck}
          <div class="modal-backdrop" on:click|self={closeLocalMkvCheck}>
            <div class="modal-card" style="max-width:600px">
              <div class="modal-title">🔍 Vérification MKV local</div>
              <div class="modal-hint">{localMkvCheck.filename}</div>
              {#if localMkvCheck.error}
                <div style="color:#ff6b6b;font-size:12px;margin:10px 0">⚠ {localMkvCheck.error}</div>
              {:else if localMkvCheck.parsed}
                <div style="display:grid;grid-template-columns:max-content 1fr;gap:6px 12px;margin:10px 0;font-size:12px">
                  <span style="color:var(--text3)">Titre détecté</span><span>{localMkvCheck.parsed.title || '—'}</span>
                  {#if localMkvCheck.parsed.year}<span style="color:var(--text3)">Année</span><span>{localMkvCheck.parsed.year}</span>{/if}
                  {#if localMkvCheck.parsed.season || localMkvCheck.parsed.episode}<span style="color:var(--text3)">S/E</span><span>S{String(localMkvCheck.parsed.season||0).padStart(2,'0')}E{String(localMkvCheck.parsed.episode||0).padStart(2,'0')}</span>{/if}
                </div>
                {#if localMkvCheck.hydrackerFiche}
                  <div style="background:rgba(0,180,216,0.05);border:1px solid rgba(0,180,216,0.25);border-radius:8px;padding:10px;margin:8px 0">
                    <div style="font-weight:600;font-size:13px">✓ Fiche : {localMkvCheck.hydrackerFiche.name} (#{localMkvCheck.hydrackerFiche.id})</div>
                    {#if localMkvCheck.content}
                      <div style="display:flex;gap:14px;margin-top:8px;font-size:12px">
                        <span>📦 Torrents : <b>{localMkvCheck.content.torrents?.torrents?.length || 0}</b></span>
                        <span>📰 NZB : <b>{localMkvCheck.content.nzbs?.nzbs?.length || 0}</b></span>
                        <span>🔗 DDL : <b>{localMkvCheck.content.liens?.liens?.length || 0}</b></span>
                      </div>
                      {#if localMkvCheck.content.liens?.liens?.length}
                        <div style="color:#7ef0c0;font-size:12px;margin-top:8px">✓ DDL dispo — auto-reseed DDL→FTP possible</div>
                      {/if}
                    {/if}
                  </div>
                  <div class="post-actions" style="flex-wrap:wrap;gap:6px">
                    {#if localMkvCheck.content?.liens?.liens?.length && cfg.ftp_host && cfg.one_fichier_api_key}
                      <button class="btn-save" style="background:#7ef0c0;color:#000"
                        on:click={async () => {
                          const fid = localMkvCheck.hydrackerFiche.id
                          const sa = localMkvCheck.parsed?.season || 0
                          const ep = localMkvCheck.parsed?.episode || 0
                          closeLocalMkvCheck()
                          autoReseedInput = String(fid)
                          autoReseedSaison = sa
                          autoReseedEpisode = ep
                          activeTab = 'reseed'
                          setTimeout(() => launchAutoReseedDDL(), 300)
                        }}
                        title="Téléchargement DDL 1fichier → streaming direct vers FTP configuré">
                        ⚡ Lancer DDL → FTP
                      </button>
                    {:else if localMkvCheck.content?.liens?.liens?.length}
                      <span style="color:var(--text3);font-size:11px;align-self:center">💡 Configure FTP + clé API 1fichier pour activer DDL→FTP</span>
                    {/if}
                    {#if localMkvCheck.content?.torrents?.torrents?.length && cfg.seedbox_url}
                      <button class="btn-save"
                        on:click={async () => {
                          const fid = localMkvCheck.hydrackerFiche.id
                          const sa = localMkvCheck.parsed?.season || 0
                          const ep = localMkvCheck.parsed?.episode || 0
                          closeLocalMkvCheck()
                          autoReseedInput = String(fid)
                          autoReseedSaison = sa
                          autoReseedEpisode = ep
                          activeTab = 'reseed'
                          setTimeout(() => launchAutoReseed(), 300)
                        }}
                        title="Auto-reseed via torrent Hydracker → seedbox">
                        ⚡ Torrent → seedbox
                      </button>
                    {/if}
                    <button class="btn-save" on:click={() => { activeTab = 'fiches'; fichesMode = 'hydracker_id'; fichesQuery = String(localMkvCheck.hydrackerFiche.id); fichesSearch(); closeLocalMkvCheck() }}>
                      🎞 Ouvrir dans Fiches
                    </button>
                    <button class="btn-test" on:click={closeLocalMkvCheck}>Fermer</button>
                  </div>
                {/if}
              {/if}
              {#if localMkvChecking}
                <div style="color:var(--text3);font-size:12px;margin-top:10px">Recherche…</div>
              {/if}
            </div>
          </div>
        {/if}

        <!-- Section 2 : Cross-check seedbox local + LiHDL (legacy, optionnelle) -->
        <div class="section">
          <div class="section-header">
            <span>📂 Seedbox locale (cross-check LiHDL)</span>
            <button class="btn-test" on:click={() => loadCheckTorrents(false)} disabled={checkLoading}>
              {checkLoading ? '…' : 'Charger seedbox'}
            </button>
          </div>
        {#if checkTorrents.length > 0}
          <div class="check-filters">
            <button class="filter-btn" class:active={checkFilter === 'all'} on:click={() => checkFilter = 'all'}>
              Tous <span class="filter-count">{checkTorrents.length}</span>
            </button>
            <button class="filter-btn" class:active={checkFilter === 'active'} on:click={() => checkFilter = 'active'}>
              Actif <span class="filter-count">{checkTorrents.filter(t => t.is_active === 1).length}</span>
            </button>
            <button class="filter-btn" class:active={checkFilter === 'inactive'} on:click={() => checkFilter = 'inactive'}>
              Inactif <span class="filter-count">{checkTorrents.filter(t => isIncomplete(t)).length}</span>
            </button>
          </div>
        {/if}
        {#if !checkTorrents.length && !checkLoading}
          <p class="coming-soon">Clique sur <b>Charger</b> pour lister les torrents de la seedbox.</p>
        {:else}
          {#each filteredCheckTorrents as t (t.hash)}
            {@const st = checkState[t.hash] || {}}
            <div class="chk-card" class:err={t.has_error} class:ok={!t.has_error}>
              <div class="chk-head">
                <div class="chk-name" title={t.name}>{t.name}</div>
                {#if t.has_error}
                  <span class="chk-badge err">⚠ Erreur</span>
                {:else if t.is_active}
                  <span class="chk-badge ok">✓ Actif</span>
                {:else}
                  <span class="chk-badge">Inactif</span>
                {/if}
              </div>
              {#if t.message}
                <div class="chk-msg">{t.message}</div>
              {/if}
              {#if t.file_name}
                <div class="chk-file">📄 {t.file_name}</div>
              {/if}
              {#if t.lihdl_url}
                <div class="chk-lihdl">
                  <span>✓ Match LiHDL : <code>{t.lihdl_name}</code></span>
                </div>
                {#if st.stage && st.stage !== 'done'}
                  <div class="chk-progress">
                    <div class="chk-status">{st.msg}{#if st.speed} · {st.speed.toFixed(1)} MB/s{/if}</div>
                    {#if st.percent >= 0}
                      <div class="progress-bar"><div class="progress-fill" style="width:{st.percent}%"></div></div>
                      <span class="chk-pct">{(st.percent || 0).toFixed(0)}%</span>
                    {/if}
                  </div>
                {:else if st.stage === 'done'}
                  <div class="chk-done">✓ {st.msg}</div>
                {:else}
                  <button class="btn-save chk-btn" on:click={() => reseedOne(t)}>
                    ⬇ Télécharger et re-seed
                  </button>
                {/if}
              {:else}
                <div class="chk-nomatch">Pas de MKV correspondant sur LiHDL</div>
              {/if}
            </div>
          {/each}
        {/if}
        </div>
      </div>

    <!-- ===== RÉGLAGES (fusion API + config) ===== -->
    {:else if activeTab === 'settings'}
      <div class="tab-content">
        <h2>Réglages</h2>
        <div class="sections">

          <!-- ===== Clés API ===== -->
          <div style="font-size:11px;color:var(--text3);margin:0 0 8px;text-transform:uppercase;letter-spacing:0.5px">🔑 Clés API</div>

          <div class="section section-locked">
            <div class="section-header">
              <span>🔒 Hydracker (verrouillé team)</span>
              <button class="btn-test" on:click={() => runTest('hydracker', () => TestHydracker(cfg.hydracker_base_url, cfg.hydracker_token))}>
                {#if testLoading.hydracker}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.hydracker}
              <div class="test-result" class:ok={testResults.hydracker.ok}>{testResults.hydracker.message}</div>
            {/if}
            <div class="field">
              <label>URL de base</label>
              <input type="password" value={cfg.hydracker_base_url} disabled readonly />
              <div class="field-hint">URL définie au build — non modifiable.</div>
            </div>
            <div class="field token-unlocked">
              <label>Token d'accès <span style="color:#7ef0c0;font-size:10px;font-weight:600">🔓 perso — éditable</span></label>
              <input type="password" bind:value={cfg.hydracker_token} placeholder="Bearer token" />
              <div class="field-hint">Chaque user met son propre token Hydracker.</div>
            </div>
          </div>

          <div class="section section-locked">
            <div class="section-header">
              <span>🔒 TMDB (verrouillé team)</span>
              <button class="btn-test" on:click={() => runTest('tmdb', () => TestTMDB(''))}>
                {#if testLoading.tmdb}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.tmdb}
              <div class="test-result" class:ok={testResults.tmdb.ok}>{testResults.tmdb.message}</div>
            {/if}
            <div style="color:var(--text3);font-size:12px;line-height:1.5;margin-bottom:8px">
              ✅ Recherche TMDB via le proxy configuré — pas de clé requise.<br>
              Bonus : notes IMDb fusionnées + lookup par IMDb ID + watch providers (Netflix/Disney+…).
            </div>
            <div class="field">
              <label>Index de recherche TMDB (proxy URL)</label>
              <input type="password" value={cfg.tmdb_proxy_url} disabled readonly />
              <div class="field-hint">URL imposée par la team — non modifiable.</div>
            </div>
            <div class="field">
              <label>Index de recherche LiHDL (par nom de fichier)</label>
              <input type="password" value={cfg.media_search_url} disabled readonly />
              <div class="field-hint">Endpoint custom imposé par la team — non modifiable.</div>
            </div>
            <div class="field">
              <label>Index de recherche TEAM</label>
              <input type="password" value={cfg.lihdl_base_url} disabled readonly />
              <div class="field-hint">Dossier LiHDL team-shared — verrouillé par la team.</div>
            </div>
            <div class="field">
              <label>Clé API TMDB (fallback)</label>
              <input type="password" value={cfg.tmdb_api_key} disabled readonly placeholder="— non configurée —" />
              <div class="field-hint">Non utilisée tant que le proxy fonctionne. Imposée par la team.</div>
            </div>
          </div>

          <div class="section">
            <div class="section-header">
              <span>1Fichier</span>
              <button class="btn-test" on:click={() => runTest('onefichier', () => TestOneFichier(cfg.one_fichier_api_key))}>
                {#if testLoading.onefichier}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.onefichier}
              <div class="test-result" class:ok={testResults.onefichier.ok}>{testResults.onefichier.message}</div>
            {/if}
            <div class="field">
              <label>Clé API 1Fichier</label>
              <input type="password" bind:value={cfg.one_fichier_api_key} placeholder="API key" />
            </div>
          </div>

          <div class="section">
            <div class="section-header">
              <span>Send.now</span>
              <button class="btn-test" on:click={() => runTest('sendcm', () => TestSendCm(cfg.sendcm_api_key))}>
                {#if testLoading.sendcm}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.sendcm}
              <div class="test-result" class:ok={testResults.sendcm.ok}>{testResults.sendcm.message}</div>
            {/if}
            <div class="field">
              <label>Clé API Send.now</label>
              <input type="password" bind:value={cfg.sendcm_api_key} placeholder="API key" />
            </div>
          </div>

          <!-- ===== Configuration ===== -->
          <div style="font-size:11px;color:var(--text3);margin:18px 0 8px;text-transform:uppercase;letter-spacing:0.5px">⚙️ Configuration</div>

          <!-- Usenet -->
          <div class="section">
            <div class="section-header">
              <span>Usenet</span>
              <button class="btn-test" on:click={() => runTest('usenet', () => TestUsenet(cfg.usenet_host, cfg.usenet_port))}>
                {#if testLoading.usenet}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.usenet}
              <div class="test-result" class:ok={testResults.usenet.ok}>{testResults.usenet.message}</div>
            {/if}
            <div class="fields-grid">
              <div class="field">
                <label>Serveur</label>
                <input type="text" bind:value={cfg.usenet_host} placeholder="news.example.com" />
              </div>
              <div class="field">
                <label>Port</label>
                <input type="number" bind:value={cfg.usenet_port} />
              </div>
              <div class="field">
                <label>Utilisateur</label>
                <input type="text" bind:value={cfg.usenet_user} />
              </div>
              <div class="field">
                <label>Mot de passe</label>
                <input type="password" bind:value={cfg.usenet_password} />
              </div>
              <div class="field">
                <label>Connexions</label>
                <input type="number" bind:value={cfg.usenet_connections} />
              </div>
              <div class="field field-checkbox">
                <label>
                  <input type="checkbox" bind:checked={cfg.usenet_ssl} />
                  SSL/TLS
                </label>
              </div>
              <div class="field">
                <label for="usenet-group">Newsgroup</label>
                <input id="usenet-group" type="text" bind:value={cfg.usenet_group} placeholder="alt.binaries.test" />
              </div>
            </div>
          </div>

          <!-- Watch folder -->
          <div class="section">
            <div class="section-header">
              <span>Dossier surveillé (auto-post)</span>
              {#if watchRunning}
                <button class="btn-test" style="color:#ff9585" on:click={async () => { try { await StopWatchFolder() } catch(e){} }}>■ Arrêter</button>
              {:else}
                <button class="btn-test" on:click={async () => { try { await StartWatchFolder(cfg.watch_folder) } catch(e){ addLog('WATCH', '✗ ' + e) } }}>▶ Démarrer</button>
              {/if}
            </div>
            <div class="field">
              <label for="watch-folder">Chemin</label>
              <input id="watch-folder" type="text" bind:value={cfg.watch_folder} placeholder="/Users/gandalf/Desktop/LiHDL" />
              <div class="field-hint">Chaque nouveau .mkv/.mp4 dans ce dossier déclenche l'analyse (et l'auto-post si Full Auto est activé).</div>
            </div>
            <div class="field field-checkbox">
              <label>
                <input type="checkbox" bind:checked={cfg.watch_auto_start} />
                Démarrer automatiquement au lancement de l'app
              </label>
            </div>
            {#if watchRunning}
              <div class="test-result ok">● Surveillance active sur : {cfg.watch_folder}</div>
            {/if}
          </div>



          <!-- ParPar -->
          <div class="section">
            <div class="section-header"><span>ParPar (PAR2)</span></div>
            <div class="fields-grid">
              <div class="field">
                <label for="parpar-redundancy">Redondance (%)</label>
                <input id="parpar-redundancy" type="number" bind:value={cfg.parpar_redundancy} min="1" max="100" step="1" />
              </div>
              <div class="field">
                <label for="parpar-threads">Threads</label>
                <input id="parpar-threads" type="number" bind:value={cfg.parpar_threads} min="1" max="64" />
              </div>
              <div class="field">
                <label for="parpar-slice">Slice size (octets)</label>
                <input id="parpar-slice" type="number" bind:value={cfg.parpar_slice_size} step="1024" />
              </div>
            </div>
          </div>

          <!-- Torrent (tracker URL + piece size) — verrouillé, imposé au build -->
          <div class="section">
            <div class="section-header"><span>🔒 Torrent (verrouillé)</span></div>
            <div class="field">
              <label for="tracker-url">URL tracker Hydracker (announce)</label>
              <input id="tracker-url" type="text" value={cfg.tracker_url} disabled readonly />
            </div>
            <div class="field">
              <label for="torrent-piece">Piece size (octets)</label>
              <input id="torrent-piece" type="number" value={cfg.torrent_piece_size} disabled readonly />
              <div class="field-hint">Valeur imposée par la team — non modifiable.</div>
            </div>
          </div>

          <!-- FTP RUTORRENT -->
          <div class="section">
            <div class="section-header">
              <span>FTP Rutorrent</span>
              <button class="btn-test" on:click={() => runTest('ftp', () => TestFTP(cfg.ftp_host, cfg.ftp_port, cfg.ftp_user, cfg.ftp_password))}>
                {#if testLoading.ftp}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.ftp}
              <div class="test-result" class:ok={testResults.ftp.ok}>{testResults.ftp.message}</div>
            {/if}
            <div class="fields-grid">
              <div class="field">
                <label>Hôte</label>
                <input type="text" bind:value={cfg.ftp_host} placeholder="ftp.example.com" />
              </div>
              <div class="field">
                <label>Port</label>
                <input type="number" bind:value={cfg.ftp_port} />
              </div>
              <div class="field">
                <label>Utilisateur</label>
                <input type="text" bind:value={cfg.ftp_user} />
              </div>
              <div class="field">
                <label>Mot de passe</label>
                <input type="password" bind:value={cfg.ftp_password} />
              </div>
              <div class="field">
                <label>Dossier distant</label>
                <input type="text" bind:value={cfg.ftp_path} placeholder="/" />
              </div>
            </div>
          </div>

          <!-- Seedbox -->
          <div class="section">
            <div class="section-header">
              <span>Seedbox Rutorrent</span>
              <button class="btn-test" on:click={() => runTest('seedbox', () => TestSeedbox(cfg.seedbox_url, cfg.seedbox_user, cfg.seedbox_password))}>
                {#if testLoading.seedbox}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.seedbox}
              <div class="test-result" class:ok={testResults.seedbox.ok}>{testResults.seedbox.message}</div>
            {/if}
            <div class="field">
              <label for="seedbox-url">URL ruTorrent</label>
              <input id="seedbox-url" type="text" bind:value={cfg.seedbox_url} placeholder="https://my-seedbox.example/seedbox-XXXX/rutorrent/" />
            </div>
            <div class="fields-grid">
              <div class="field">
                <label>Utilisateur</label>
                <input type="text" bind:value={cfg.seedbox_user} />
              </div>
              <div class="field">
                <label>Mot de passe</label>
                <input type="password" bind:value={cfg.seedbox_password} />
              </div>
            </div>
            <div class="field">
              <label for="seedbox-label">Label (optionnel)</label>
              <input id="seedbox-label" type="text" bind:value={cfg.seedbox_label} placeholder="hydracker" />
            </div>
          </div>

          <!-- FTP MODÉRATEUR (upload gros fichiers MKV pour le workflow Torrent MODO) -->
          <div class="section section-locked">
            <div class="section-header">
              <span>🔒 FTP Modérateur (verrouillé team)</span>
              <button class="btn-test" on:click={() => runTest('ftpmod', () => TestFTP(cfg.ftp_mod_host, cfg.ftp_mod_port, cfg.ftp_mod_user, cfg.ftp_mod_password))}>
                {#if testLoading.ftpmod}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.ftpmod}
              <div class="test-result" class:ok={testResults.ftpmod.ok}>{testResults.ftpmod.message}</div>
            {/if}
            <div style="color:var(--text3);font-size:11px;margin-bottom:8px">
              FTP/SFTP de la seedbox modérateur (utilisé par le workflow Torrent MODO pour uploader le MKV).
            </div>
            <div class="fields-grid">
              <div class="field">
                <label>Hôte</label>
                <input type="password" value={cfg.ftp_mod_host} disabled readonly />
              </div>
              <div class="field">
                <label>Port</label>
                <input type="password" value={cfg.ftp_mod_port} disabled readonly />
              </div>
              <div class="field">
                <label>Utilisateur</label>
                <input type="password" value={cfg.ftp_mod_user} disabled readonly />
              </div>
              <div class="field">
                <label>Mot de passe</label>
                <input type="password" value={cfg.ftp_mod_password} disabled readonly />
              </div>
              <div class="field">
                <label>Dossier distant</label>
                <input type="password" value={cfg.ftp_mod_path} disabled readonly />
              </div>
            </div>
          </div>

          <!-- Seedbox MODÉRATEUR (qBittorrent shared team) -->
          <div class="section section-locked">
            <div class="section-header">
              <span>🔒 Seedbox Modérateur — qBittorrent (verrouillé team)</span>
              <button class="btn-test" on:click={() => runTest('qbit', () => TestQBit(cfg.qbit_url, cfg.qbit_user, cfg.qbit_password))}>
                {#if testLoading.qbit}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.qbit}
              <div class="test-result" class:ok={testResults.qbit.ok}>{testResults.qbit.message}</div>
            {/if}
            <div class="field">
              <label for="qbit-url">URL qBittorrent Web UI</label>
              <input id="qbit-url" type="password" value={cfg.qbit_url} disabled readonly />
            </div>
            <div class="fields-grid">
              <div class="field">
                <label>Utilisateur</label>
                <input type="password" value={cfg.qbit_user} disabled readonly />
              </div>
              <div class="field">
                <label>Mot de passe</label>
                <input type="password" value={cfg.qbit_password} disabled readonly />
              </div>
            </div>
          </div>

          <!-- FTP Privé (pour le workflow "Torrent Privé" — chacun son FTP) -->
          <div class="section">
            <div class="section-header">
              <span>🏠 FTP Privé</span>
              <button class="btn-test" on:click={() => runTest('ftppriv', () => TestFTP(cfg.private_ftp_host, cfg.private_ftp_port, cfg.private_ftp_user, cfg.private_ftp_password))}>
                {#if testLoading.ftppriv}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.ftppriv}
              <div class="test-result" class:ok={testResults.ftppriv.ok}>{testResults.ftppriv.message}</div>
            {/if}
            <div style="color:var(--text3);font-size:11px;margin-bottom:8px">
              TON FTP perso (si tu utilises "Torrent Privé" au lieu de "Torrent ADMIN"). Jamais écrasé par le build.
            </div>
            <div class="fields-grid">
              <div class="field">
                <label for="pftp-host">Host</label>
                <input id="pftp-host" type="text" bind:value={cfg.private_ftp_host} placeholder="ftp.monserveur.com" />
              </div>
              <div class="field">
                <label for="pftp-port">Port</label>
                <input id="pftp-port" type="number" bind:value={cfg.private_ftp_port} />
              </div>
              <div class="field">
                <label for="pftp-user">Utilisateur</label>
                <input id="pftp-user" type="text" bind:value={cfg.private_ftp_user} />
              </div>
              <div class="field">
                <label for="pftp-pass">Mot de passe</label>
                <input id="pftp-pass" type="password" bind:value={cfg.private_ftp_password} />
              </div>
              <div class="field">
                <label for="pftp-path">Path distant</label>
                <input id="pftp-path" type="text" bind:value={cfg.private_ftp_path} placeholder="/ ou /downloads/" />
              </div>
            </div>
          </div>

          <!-- Seedbox Privée — ruTorrent OU qBittorrent (au choix) -->
          <div class="section">
            <div class="section-header">
              <span>🏠 Seedbox Privée (ruTorrent / qBittorrent)</span>
            </div>
            <div style="color:var(--text3);font-size:11px;margin-bottom:8px">
              TA seedbox perso (pour le workflow "Torrent Privé"). Renseigne <b>l'un OU l'autre</b> selon ton client. Jamais écrasée par le build.
            </div>

            <div style="display:grid;grid-template-columns:1fr 1fr;gap:14px">
              <!-- ruTorrent -->
              <div style="background:rgba(255,255,255,0.02);border:1px solid var(--border);border-radius:8px;padding:10px">
                <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
                  <b style="font-size:12px">ruTorrent</b>
                  <button class="btn-test" on:click={() => runTest('seedboxpriv', () => TestSeedbox(cfg.private_seedbox_url, cfg.private_seedbox_user, cfg.private_seedbox_password))}>
                    {#if testLoading.seedboxpriv}…{:else}Tester{/if}
                  </button>
                </div>
                {#if testResults.seedboxpriv}
                  <div class="test-result" class:ok={testResults.seedboxpriv.ok}>{testResults.seedboxpriv.message}</div>
                {/if}
                <div class="field">
                  <label for="pseedbox-url">URL ruTorrent</label>
                  <input id="pseedbox-url" type="text" bind:value={cfg.private_seedbox_url} placeholder="https://serveur/rutorrent/" />
                </div>
                <div class="field">
                  <label for="pseedbox-user">Utilisateur</label>
                  <input id="pseedbox-user" type="text" bind:value={cfg.private_seedbox_user} />
                </div>
                <div class="field">
                  <label for="pseedbox-pass">Mot de passe</label>
                  <input id="pseedbox-pass" type="password" bind:value={cfg.private_seedbox_password} />
                </div>
                <div class="field">
                  <label for="pseedbox-label">Label (optionnel)</label>
                  <input id="pseedbox-label" type="text" bind:value={cfg.private_seedbox_label} />
                </div>
              </div>

              <!-- qBittorrent -->
              <div style="background:rgba(255,255,255,0.02);border:1px solid var(--border);border-radius:8px;padding:10px">
                <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
                  <b style="font-size:12px">qBittorrent</b>
                  <button class="btn-test" on:click={() => runTest('qbitpriv', () => TestQBit(cfg.private_qbit_url, cfg.private_qbit_user, cfg.private_qbit_password))}>
                    {#if testLoading.qbitpriv}…{:else}Tester{/if}
                  </button>
                </div>
                {#if testResults.qbitpriv}
                  <div class="test-result" class:ok={testResults.qbitpriv.ok}>{testResults.qbitpriv.message}</div>
                {/if}
                <div class="field">
                  <label for="pqbit-url">URL Web UI</label>
                  <input id="pqbit-url" type="text" bind:value={cfg.private_qbit_url} placeholder="https://serveur:8080/" />
                </div>
                <div class="field">
                  <label for="pqbit-user">Utilisateur</label>
                  <input id="pqbit-user" type="text" bind:value={cfg.private_qbit_user} />
                </div>
                <div class="field">
                  <label for="pqbit-pass">Mot de passe</label>
                  <input id="pqbit-pass" type="password" bind:value={cfg.private_qbit_password} />
                </div>
                <div style="font-size:10px;color:var(--text3);font-style:italic;margin-top:8px">
                  qBit prioritaire si configuré, sinon ruTorrent.
                </div>
              </div>
            </div>
          </div>

        </div>

        <button class="btn-save" on:click={saveConfig}>
          {cfgSaved ? '✓ Sauvegardé' : 'Sauvegarder'}
        </button>
      </div>

    <!-- ===== JOURNAL ===== -->
    {:else if activeTab === 'log'}
      <div class="tab-content">
        <h2>Journal d'activité</h2>
        {#if $logEntries.length === 0}
          <p class="coming-soon">Les logs en temps réel apparaîtront ici</p>
        {:else}
          <div class="log-toolbar">
            <button class="btn-log-clear" on:click={clearLogs}>Effacer</button>
          </div>
          <div class="log-list">
            {#each $logEntries as e}
              <div class="log-line">
                <span class="log-ts">{e.ts}</span>
                <span class="log-prefix log-prefix-{e.prefix.toLowerCase()}">{e.prefix}</span>
                <span class="log-msg">{e.msg}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

    {/if}<!-- fin activeTab !== hydracker -->

  </main>
</div>

<!-- ===== Modale mot de passe LiHDL ===== -->
{#if lihdlModal}
  <div class="modal-backdrop" on:click|self={() => lihdlModal = null}>
    <div class="modal-card">
      <div class="modal-header">
        <h3>
          {#if lihdlModal === 'unlock'}🔓 Déverrouiller LiHDL
          {:else if lihdlModal === 'create'}🔐 Définir un mot de passe
          {:else if lihdlModal === 'change'}🔑 Changer le mot de passe
          {:else}🗑 Retirer la protection{/if}
        </h3>
        <button class="modal-close" on:click={() => lihdlModal = null}>✕</button>
      </div>
      <div class="modal-body">
        <form on:submit|preventDefault={submitLihdlPwd}>
          {#if lihdlModal === 'unlock'}
            <div class="field"><label>Mot de passe</label>
              <input type="password" bind:value={lihdlPwdInput} autofocus autocomplete="off" /></div>
          {:else if lihdlModal === 'create'}
            <div class="field"><label>Nouveau mot de passe</label>
              <input type="password" bind:value={lihdlPwdNew} autofocus autocomplete="off" /></div>
            <div class="field"><label>Confirmer</label>
              <input type="password" bind:value={lihdlPwdConfirm} autocomplete="off" /></div>
          {:else if lihdlModal === 'change'}
            <div class="field"><label>Mot de passe actuel</label>
              <input type="password" bind:value={lihdlPwdCurrent} autofocus autocomplete="off" /></div>
            <div class="field"><label>Nouveau mot de passe</label>
              <input type="password" bind:value={lihdlPwdNew} autocomplete="off" /></div>
            <div class="field"><label>Confirmer</label>
              <input type="password" bind:value={lihdlPwdConfirm} autocomplete="off" /></div>
          {:else}
            <div class="field"><label>Mot de passe actuel</label>
              <input type="password" bind:value={lihdlPwdCurrent} autofocus autocomplete="off" /></div>
            <p class="modal-hint">La section redeviendra accessible sans mot de passe.</p>
          {/if}
          {#if lihdlPwdError}<p class="modal-err">✗ {lihdlPwdError}</p>{/if}
          <div class="modal-actions" style="margin-top:12px">
            <button type="button" class="btn-secondary" on:click={() => lihdlModal = null}>Annuler</button>
            <button type="submit" class="btn-primary">Valider</button>
          </div>
        </form>
      </div>
    </div>
  </div>
{/if}

<!-- ===== Modale mot de passe SEEDBOX (FTP+seedbox sections) ===== -->
{#if seedboxModal}
  <div class="modal-backdrop" on:click|self={() => seedboxModal = null}>
    <div class="modal-card">
      <div class="modal-header">
        <h3>
          {#if seedboxModal === 'unlock'}🔓 Déverrouiller Seedbox/FTP
          {:else if seedboxModal === 'create'}🔐 Définir un mot de passe
          {:else if seedboxModal === 'change'}🔑 Changer le mot de passe
          {:else}🗑 Retirer la protection{/if}
        </h3>
        <button class="modal-close" on:click={() => seedboxModal = null}>✕</button>
      </div>
      <div class="modal-body">
        <form on:submit|preventDefault={submitSeedboxPwd}>
          {#if seedboxModal === 'unlock'}
            <div class="field"><label>Mot de passe</label>
              <input type="password" bind:value={seedboxPwdInput} autofocus autocomplete="off" /></div>
          {:else if seedboxModal === 'create'}
            <p class="modal-hint">Mot de passe partagé entre les admins. Donne-le aux 3 autres pour qu'ils puissent déverrouiller leurs sections aussi.</p>
            <div class="field"><label>Nouveau mot de passe</label>
              <input type="password" bind:value={seedboxPwdNew} autofocus autocomplete="off" /></div>
            <div class="field"><label>Confirmer</label>
              <input type="password" bind:value={seedboxPwdConfirm} autocomplete="off" /></div>
          {:else if seedboxModal === 'change'}
            <div class="field"><label>Mot de passe actuel</label>
              <input type="password" bind:value={seedboxPwdCurrent} autofocus autocomplete="off" /></div>
            <div class="field"><label>Nouveau mot de passe</label>
              <input type="password" bind:value={seedboxPwdNew} autocomplete="off" /></div>
            <div class="field"><label>Confirmer</label>
              <input type="password" bind:value={seedboxPwdConfirm} autocomplete="off" /></div>
          {:else}
            <div class="field"><label>Mot de passe actuel</label>
              <input type="password" bind:value={seedboxPwdCurrent} autofocus autocomplete="off" /></div>
            <p class="modal-hint">Les sections redeviendront accessibles sans mot de passe.</p>
          {/if}
          {#if seedboxPwdError}<p class="modal-err">✗ {seedboxPwdError}</p>{/if}
          <div class="modal-actions" style="margin-top:12px">
            <button type="button" class="btn-secondary" on:click={() => seedboxModal = null}>Annuler</button>
            <button type="submit" class="btn-primary">Valider</button>
          </div>
        </form>
      </div>
    </div>
  </div>
{/if}

<!-- ===== Modale mise à jour ===== -->
{#if showUpdateModal && updateInfo?.available}
  <div class="modal-backdrop" on:click|self={() => { if (!updateState.downloading) showUpdateModal = false }}>
    <div class="modal-card">
      <div class="modal-header">
        <h3>🆕 Mise à jour disponible</h3>
        <button class="modal-close" on:click={() => { if (!updateState.downloading) showUpdateModal = false }}>✕</button>
      </div>
      <div class="modal-body">
        <p>Version actuelle : <b>v{updateInfo.current}</b> → nouvelle : <b>v{updateInfo.latest}</b></p>
        {#if !updateState.stage}
          <p class="modal-hint">Télécharger la nouvelle version dans ton dossier Téléchargements.</p>
        {:else if updateState.downloading}
          <div class="update-progress">
            <div class="update-bar"><div class="update-bar-fill" style="width:{updateState.percent}%"></div></div>
            <div class="update-msg">{updateState.msg} · {updateState.percent.toFixed(0)}%</div>
          </div>
        {:else if updateState.stage === 'done'}
          <p class="modal-ok">✓ Téléchargé. Le Finder s'est ouvert sur le fichier. Dézippe et remplace l'app.</p>
          <p class="modal-hint" style="margin-top:8px">Chemin : <code>{updateState.downloadedPath}</code></p>
        {:else if updateState.stage === 'error'}
          <p class="modal-err">✗ Erreur : {updateState.msg}</p>
        {/if}
      </div>
      <div class="modal-actions">
        {#if !updateState.stage || updateState.stage === 'error'}
          <button class="btn-primary" on:click={startDownloadUpdate}>⬇ Télécharger</button>
          <button class="btn-secondary" on:click={() => OpenBrowser(updateInfo.url)}>Voir sur GitHub</button>
        {:else if updateState.stage === 'done'}
          <button class="btn-primary" on:click={() => showUpdateModal = false}>Fermer</button>
        {/if}
      </div>
    </div>
  </div>
{/if}

{#if nfoModal}
  <div class="nfo-modal-bg" on:click={closeNfo}>
    <div class="nfo-modal-card" on:click|stopPropagation>
      <div class="nfo-modal-head">
        <div>
          <div class="nfo-modal-title">📄 NFO — {nfoModal.kind === 'torrents' ? 'Torrent' : nfoModal.kind === 'nzbs' ? 'NZB' : 'Lien DDL'} #{nfoModal.id}</div>
          {#if nfoModal.title}<div class="nfo-modal-sub">{nfoModal.title}</div>{/if}
        </div>
        <button class="btn-test" on:click={closeNfo}>✕ Fermer</button>
      </div>
      <div class="nfo-modal-body">
        {#if nfoModal.loading}
          <div style="color:var(--text3);font-size:12px">Chargement…</div>
        {:else if nfoModal.error}
          <div style="color:#ff9585;font-size:12px">{nfoModal.error}</div>
        {:else}
          {@html nfoModal.html}
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  :global(:root) {
    color-scheme: dark;
    --bg:         #0d0a10;
    --bg-tint:    #14101a;
    --bg2:        #1a1420;
    --bg3:        #241c2a;
    --border:     rgba(255, 255, 255, 0.08);
    --border-strong: rgba(255, 255, 255, 0.14);
    --red:        #e63946;
    --red-hot:    #ff5a4a;
    --orange:     #f77f00;
    --blue:       #00b4d8;
    --blue-hot:   #48cae4;
    --yellow:     #ffd60a;
    --accent:     var(--red);
    --accent-glow: rgba(230, 57, 70, 0.45);
    --success:    #22c55e;
    --warning:    #ffd60a;
    --danger:     #ef4444;
    --text:       #f5efe7;
    --text2:      #b5a9a1;
    --text3:      #7a6e68;
    --grad-primary:       linear-gradient(180deg, #ff6b3d 0%, #e8431a 100%);
    --grad-primary-hover: linear-gradient(180deg, #ff7a4f 0%, #f14a24 100%);
  }
  :global(*) { box-sizing: border-box; margin: 0; padding: 0; }
  :global(body) {
    margin: 0;
    font-family: "SF Pro Display", -apple-system, BlinkMacSystemFont, "Segoe UI Variable", "Segoe UI", system-ui, sans-serif;
    font-size: 14px;
    color: var(--text);
    background:
      radial-gradient(ellipse 120% 80% at 50% -10%, rgba(230, 57, 70, 0.08) 0%, transparent 50%),
      radial-gradient(ellipse 80% 60% at 20% 110%, rgba(0, 180, 216, 0.06) 0%, transparent 55%),
      var(--bg);
    height: 100vh;
    overflow: hidden;
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
  }
  :global(body::before) {
    content: "";
    position: fixed;
    inset: 0;
    pointer-events: none;
    z-index: 1;
    opacity: 0.035;
    background-image: url("data:image/svg+xml;utf8,<svg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='3' stitchTiles='stitch'/></filter><rect width='100%25' height='100%25' filter='url(%23n)'/></svg>");
    mix-blend-mode: overlay;
  }
  :global(input[type=text]),
  :global(input[type=password]),
  :global(input[type=number]),
  :global(input[type=email]),
  :global(select) {
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    padding: 8px 11px;
    font: inherit;
    font-size: 13px;
    outline: none;
    width: 100%;
    transition: border-color 140ms, background 140ms;
  }
  :global(input:focus), :global(select:focus) {
    border-color: rgba(0, 180, 216, 0.45);
    background: rgba(255,255,255,0.05);
  }
  :global(button) { cursor: pointer; border: none; border-radius: 8px; font-size: 13px; font-weight: 500; transition: all 140ms ease; }
  :global(::-webkit-scrollbar) { width: 6px; height: 6px; }
  :global(::-webkit-scrollbar-track) { background: transparent; }
  :global(::-webkit-scrollbar-thumb) { background: rgba(255,255,255,0.1); border-radius: 3px; }
  :global(::-webkit-scrollbar-thumb:hover) { background: rgba(255,255,255,0.2); }

  .layout { display: flex; height: 100vh; position: relative; z-index: 2; }

  .sidebar {
    width: 200px; min-width: 200px;
    background: rgba(20, 16, 26, 0.6);
    backdrop-filter: blur(12px);
    border-right: 1px solid var(--border);
    display: flex; flex-direction: column;
    padding: 18px 12px;
    position: relative;
    transition: width 0.2s ease, min-width 0.2s ease, padding 0.2s ease;
  }
  .sidebar.collapsed { width: 56px; min-width: 56px; padding: 18px 6px; }
  .sidebar.collapsed .brand { margin-bottom: 14px; }
  .sidebar.collapsed .brand-logo { width: 36px; }
  .sidebar.collapsed .nav-item {
    padding: 8px 4px; font-size: 16px; text-align: center;
    white-space: nowrap; overflow: hidden;
  }
  .sidebar.collapsed .btn-update { padding: 8px 4px; font-size: 14px; }

  .sidebar-toggle {
    position: absolute; top: 14px; right: -12px; z-index: 5;
    width: 24px; height: 24px; border-radius: 50%;
    background: var(--bg2); border: 1px solid var(--border);
    color: var(--text2); cursor: pointer;
    display: flex; align-items: center; justify-content: center;
    font-size: 16px; line-height: 1; font-weight: 700;
    box-shadow: 0 2px 8px rgba(0,0,0,0.3);
  }
  .sidebar-toggle:hover { background: var(--bg3); color: var(--text); }
  .brand {
    display: flex; flex-direction: column; align-items: center;
    gap: 10px; margin-bottom: 24px; padding: 4px 6px;
  }
  .brand-logo {
    width: 78px; height: auto;
    filter: drop-shadow(0 0 14px rgba(230, 57, 70, 0.45));
    user-select: none;
    -webkit-user-drag: none;
  }
  .logo {
    font-size: 12px; font-weight: 700;
    letter-spacing: 2.4px;
    text-transform: uppercase;
    background: linear-gradient(135deg, #ff6b3d 0%, #ffd60a 55%, #00b4d8 100%);
    -webkit-background-clip: text;
    background-clip: text;
    -webkit-text-fill-color: transparent;
    user-select: none;
    text-align: center;
  }
  nav { display: flex; flex-direction: column; gap: 3px; }
  .nav-item {
    text-align: left; padding: 10px 13px; border-radius: 9px;
    background: transparent; color: var(--text2); font-size: 13px; width: 100%;
    transition: all 160ms ease;
  }
  .nav-item:hover { background: rgba(255,255,255,0.04); color: var(--text); }
  .nav-item.active {
    background: rgba(0, 180, 216, 0.08);
    color: var(--text);
    font-weight: 600;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.05);
    border: 1px solid rgba(0, 180, 216, 0.2);
  }

  .content { flex: 1; overflow-y: auto; }

  .hist-stats { display:flex; flex-wrap:wrap; gap:8px; margin-bottom:14px; }
  .hist-stat { padding:5px 12px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.08); border-radius:14px; font-size:12px; color:var(--text2); }
  .hist-stat b { color:var(--text1); margin-left:4px; }
  .hist-stat.ok b { color:#22c55e; }
  .hist-stat.err b { color:#ef4444; }
  .hist-filters { display:flex; gap:10px; margin-bottom:14px; align-items:center; flex-wrap:wrap; }
  .hist-search { flex:1; min-width:260px; padding:8px 12px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.1); border-radius:8px; color:var(--text1); font-size:13px; }
  .hist-type-btns { display:flex; gap:4px; }
  .hist-type-btns button { padding:6px 12px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.08); border-radius:8px; color:var(--text2); font-size:12px; cursor:pointer; }
  .hist-type-btns button.active { background:rgba(255,90,60,0.15); border-color:#ff5a3c; color:#ff5a3c; }
  .hist-list { display:flex; flex-direction:column; gap:6px; max-height:calc(100vh - 260px); overflow-y:auto; }
  .hist-row { display:grid; grid-template-columns:110px 70px 1fr 80px 24px; gap:12px; padding:10px 14px; background:rgba(255,255,255,0.03); border:1px solid rgba(255,255,255,0.06); border-radius:8px; align-items:start; }
  .hist-row.err { border-color:rgba(239,68,68,0.3); background:rgba(239,68,68,0.05); }
  .hist-col-date { font-size:11px; color:var(--text3); font-family:monospace; }
  .hist-col-type { font-size:10px; font-weight:700; letter-spacing:0.5px; padding:2px 6px; border-radius:4px; text-align:center; align-self:start; }
  .hist-type-torrent { background:rgba(34,197,94,0.15); color:#22c55e; }
  .hist-type-nzb { background:rgba(59,130,246,0.15); color:#3b82f6; }
  .hist-type-ddl { background:rgba(168,85,247,0.15); color:#a855f7; }
  .hist-col-main { min-width:0; }
  .hist-title { font-size:13px; color:var(--text1); font-weight:600; }
  .hist-sub { font-size:11px; color:var(--text3); margin-top:2px; }
  .hist-links { font-size:10px; color:#00b4d8; margin-top:4px; word-break:break-all; white-space:pre-line; }
  .hist-error { font-size:11px; color:#ef4444; margin-top:4px; }
  .hist-col-id { font-size:12px; color:var(--text2); font-family:monospace; }
  .hist-del { background:transparent; border:0; color:var(--text3); cursor:pointer; font-size:14px; padding:0; }
  .hist-del:hover { color:#ef4444; }
  .hist-empty { padding:40px; text-align:center; color:var(--text3); font-size:13px; background:rgba(255,255,255,0.02); border-radius:8px; }

  .modal-backdrop { position:fixed; inset:0; background:rgba(0,0,0,0.6); backdrop-filter:blur(6px); display:flex; align-items:center; justify-content:center; z-index:9999; }
  .modal-card { background:#151119; border:1px solid rgba(255,255,255,0.1); border-radius:12px; width:min(90vw, 520px); box-shadow:0 20px 60px rgba(0,0,0,0.5); }
  .modal-header { display:flex; align-items:center; justify-content:space-between; padding:16px 20px; border-bottom:1px solid rgba(255,255,255,0.08); }
  .modal-header h3 { margin:0; font-size:16px; color:var(--text1); }
  .modal-close { background:transparent; border:0; color:var(--text3); font-size:18px; cursor:pointer; }
  .modal-close:hover { color:var(--text1); }
  .modal-body { padding:20px; color:var(--text2); font-size:13px; line-height:1.5; }
  .modal-body p { margin:0 0 8px; }
  .modal-hint { color:var(--text3); font-size:12px; }
  .modal-ok { color:#22c55e; }
  .modal-err { color:#ef4444; }
  .modal-body code { font-family:monospace; font-size:11px; background:rgba(255,255,255,0.05); padding:2px 6px; border-radius:4px; color:var(--text1); word-break:break-all; }
  .update-progress { margin-top:12px; }
  .update-bar { height:8px; background:rgba(255,255,255,0.06); border-radius:4px; overflow:hidden; }
  .update-bar-fill { height:100%; background:linear-gradient(90deg, #ff5a3c, #ff8b6b); transition:width 0.2s; }
  .update-msg { margin-top:8px; font-size:12px; color:var(--text3); font-family:monospace; }
  .modal-actions { display:flex; gap:10px; justify-content:flex-end; padding:14px 20px; border-top:1px solid rgba(255,255,255,0.08); }
  .btn-primary { background:#ff5a3c; border:0; color:white; padding:8px 18px; border-radius:8px; font-weight:600; cursor:pointer; font-size:13px; }
  .btn-primary:hover { background:#ff6b4f; }
  .btn-secondary { background:rgba(255,255,255,0.06); border:1px solid rgba(255,255,255,0.1); color:var(--text1); padding:8px 18px; border-radius:8px; cursor:pointer; font-size:13px; }
  .btn-secondary:hover { background:rgba(255,255,255,0.1); }

  .tab-content { padding: 28px 32px; max-width: 900px; }
  .tab-content h2 {
    font-size: 18px; font-weight: 700; margin-bottom: 22px;
    color: var(--text); letter-spacing: -0.01em;
  }

  .coming-soon {
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px dashed var(--border);
    border-radius: 14px; padding: 48px;
    text-align: center; color: var(--text3); font-size: 14px;
  }

  .sections { display: flex; flex-direction: column; gap: 16px; }

  .section {
    position: relative;
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px solid var(--border);
    border-radius: 14px;
    padding: 18px 20px;
    box-shadow:
      inset 0 1px 0 rgba(255,255,255,0.05),
      0 1px 2px rgba(0,0,0,0.4);
    animation: card-in 420ms cubic-bezier(0.16, 1, 0.3, 1) both;
  }
  @keyframes card-in {
    from { opacity: 0; transform: translateY(10px); }
    to   { opacity: 1; transform: translateY(0); }
  }
  .section-header {
    display: flex; align-items: center; justify-content: space-between;
    margin-bottom: 14px;
  }
  .section-header span {
    font-weight: 600; color: var(--text2); font-size: 11px;
    text-transform: uppercase; letter-spacing: 1.2px;
  }
  /* Section verrouillée team-shared (creds bakés au build) */
  .section-locked > .section-header {
    border-bottom: 2px solid #ff4444;
    padding-bottom: 8px;
  }
  .section-locked > .section-header span {
    color: #ff4444;
  }
  .section-locked input,
  .section-locked select,
  .section-locked textarea {
    opacity: 0.5;
    cursor: not-allowed;
    background: rgba(255, 68, 68, 0.04) !important;
    border-color: rgba(255, 68, 68, 0.2) !important;
  }
  /* Exception : les champs dans .token-unlocked restent éditables malgré
     la section-locked. Utilisé pour le Token Hydracker (perso par user). */
  .section-locked .token-unlocked input,
  .section-locked .token-unlocked select,
  .section-locked .token-unlocked textarea {
    opacity: 1;
    cursor: text;
    background: rgba(126, 240, 192, 0.04) !important;
    border-color: rgba(126, 240, 192, 0.3) !important;
  }

  .btn-test {
    background: rgba(255,255,255,0.04);
    color: var(--text2);
    border: 1px solid var(--border);
    padding: 6px 14px; font-size: 11px; font-weight: 500;
    text-transform: uppercase; letter-spacing: 0.4px;
  }
  .btn-test:hover {
    background: rgba(0, 180, 216, 0.08);
    border-color: rgba(0, 180, 216, 0.35);
    color: var(--text);
  }

  .locked-box {
    padding: 16px; text-align: center;
    background: rgba(255,255,255,0.02); border: 1px dashed rgba(255,255,255,0.1);
    border-radius: 10px; color: var(--text3); font-size: 12px;
  }

  .active-chip {
    background: rgba(0, 180, 216, 0.18) !important;
    border-color: rgba(0, 180, 216, 0.55) !important;
    color: var(--text) !important;
  }
  .fiches-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
    gap: 14px;
    margin-top: 8px;
  }
  .fiche-card {
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0;
    overflow: hidden;
    cursor: pointer;
    text-align: left;
    display: flex;
    flex-direction: column;
    transition: all 0.15s;
  }
  .fiche-card:hover {
    border-color: rgba(0, 180, 216, 0.55);
    background: rgba(0, 180, 216, 0.04);
    transform: translateY(-2px);
  }
  .fiche-card img {
    width: 100%;
    aspect-ratio: 2/3;
    object-fit: cover;
    display: block;
  }
  .fiche-no-poster {
    width: 100%; aspect-ratio: 2/3;
    display: flex; align-items: center; justify-content: center;
    font-size: 48px; color: var(--text3);
    background: rgba(255,255,255,0.02);
  }
  .fiche-info { padding: 8px 10px; }
  .fiche-name { font-size: 13px; font-weight: 600; color: var(--text); margin-bottom: 3px; line-height: 1.2; }
  .fiche-meta { font-size: 11px; color: var(--text3); }
  .fiche-id { font-size: 10px; color: var(--text3); margin-top: 2px; opacity: 0.7; }

  /* === Cartes contenu (torrents/nzbs/liens) dans la fiche === */
  .content-grid { display: flex; flex-direction: column; gap: 8px; }
  .content-card {
    background: rgba(255,255,255,0.025);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 12px;
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 12px;
    transition: border-color 0.15s, background 0.15s;
  }
  .content-card:hover { border-color: rgba(0, 180, 216, 0.35); background: rgba(0, 180, 216, 0.025); }
  .cc-body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 6px; }
  .cc-head { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
  .cc-name { font-size: 12px; color: var(--text); font-weight: 500; word-break: break-all; line-height: 1.35; }
  .cc-name-mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; color: var(--text2, #b8b3c0); }
  .cc-sub-url { opacity: 0.7; }
  .cc-name-loading { font-size: 10.5px; color: var(--text3); font-style: italic; }
  .cc-tags { display: flex; flex-wrap: wrap; gap: 5px; }
  .cc-actions { display: flex; gap: 6px; align-items: center; flex-shrink: 0; }

  .cc-id { font-size: 10px; color: var(--text3); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  .cc-chip {
    display: inline-flex; align-items: center;
    padding: 2px 8px; border-radius: 10px;
    font-size: 10px; font-weight: 600; line-height: 1.4;
    border: 1px solid transparent;
  }
  .cc-chip-qual { background: rgba(249, 115, 22, 0.12); color: #fb923c; border-color: rgba(249, 115, 22, 0.35); }
  .cc-chip-se   { background: rgba(126, 240, 192, 0.10); color: #7ef0c0; border-color: rgba(126, 240, 192, 0.30); }
  .cc-chip-size { background: rgba(255,255,255,0.05); color: var(--text3); border-color: rgba(255,255,255,0.10); }
  .cc-chip-host { background: rgba(59, 130, 246, 0.12); color: #60a5fa; border-color: rgba(59, 130, 246, 0.30); }
  .cc-chip-seed { background: rgba(255,255,255,0.04); border-color: rgba(255,255,255,0.10); }

  .cc-tag {
    display: inline-flex; align-items: center; gap: 3px;
    padding: 2px 7px; border-radius: 4px;
    font-size: 10.5px; font-weight: 500;
    background: rgba(255,255,255,0.04);
    color: var(--text3);
    border: 1px solid rgba(255,255,255,0.06);
  }
  .cc-tag-lang   { color: #d4d0db; background: rgba(126, 240, 192, 0.06); border-color: rgba(126, 240, 192, 0.18); }
  .cc-tag-sub    { color: var(--text3); background: rgba(255,255,255,0.035); }
  .cc-tag-author { color: var(--text3); background: rgba(255,255,255,0.025); font-style: italic; }

  .btn-icon {
    min-width: 28px; padding: 4px 8px !important;
    font-size: 13px; font-weight: bold;
  }

  /* === Stats uploaders (table) === */
  .stats-header {
    display: flex; justify-content: space-between; align-items: flex-start;
    gap: 16px; margin: 8px 0 18px; flex-wrap: wrap;
  }
  .stats-title { margin: 0 0 4px; font-size: 16px; font-weight: 600; color: var(--text); }
  .stats-sub { color: var(--text3); font-size: 11.5px; line-height: 1.5; max-width: 720px; }

  .upl-table-wrap {
    background: rgba(255,255,255,0.02);
    border: 1px solid var(--border);
    border-radius: 10px;
    overflow: hidden;
  }
  .upl-table {
    width: 100%; border-collapse: collapse;
  }
  .upl-table thead tr {
    background: linear-gradient(180deg, rgba(255,255,255,0.06), rgba(255,255,255,0.02));
    border-bottom: 2px solid var(--border-strong);
  }
  .upl-table th {
    padding: 11px 12px;
    text-align: left;
    font-size: 11px; font-weight: 600; color: var(--text);
    letter-spacing: 0.3px;
    text-transform: uppercase;
    white-space: nowrap;
  }
  .upl-th-rank { width: 50px; text-align: center !important; }
  .upl-th-num { text-align: right !important; }
  .upl-th-sort { cursor: pointer; user-select: none; transition: background 0.12s; }
  .upl-th-sort:hover { background: rgba(255,255,255,0.04); }

  .upl-row {
    cursor: pointer;
    transition: background 0.12s;
    border-bottom: 1px solid rgba(255,255,255,0.04);
  }
  .upl-row:hover { background: rgba(0, 180, 216, 0.06); }
  .upl-row:last-child { border-bottom: none; }
  .upl-podium { background: rgba(255, 214, 10, 0.03); }
  .upl-podium:hover { background: rgba(255, 214, 10, 0.08); }

  .upl-table td {
    padding: 9px 12px;
    font-size: 12.5px; color: var(--text);
  }
  .upl-rank {
    text-align: center;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    color: var(--text3); font-weight: 700; font-size: 13px;
    width: 50px;
  }
  .upl-author {
    font-weight: 600;
  }
  .upl-num { text-align: right; font-variant-numeric: tabular-nums; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
  .upl-c-torrent { color: #fb923c; }
  .upl-c-nzb     { color: #60a5fa; }
  .upl-c-ddl     { color: #7ef0c0; }
  .upl-c-total   { font-weight: 700; color: var(--text); font-size: 13px; }
  .upl-c-size    { color: var(--text3); }
  .upl-date      { color: var(--text3); font-size: 11px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; white-space: nowrap; }

  /* === Modale NFO === */
  .nfo-modal-bg {
    position: fixed; inset: 0; z-index: 1000;
    background: rgba(0,0,0,0.75);
    display: flex; align-items: center; justify-content: center;
    padding: 24px;
  }
  .nfo-modal-card {
    background: var(--bg2); border: 1px solid var(--border-strong);
    border-radius: 12px; max-width: 880px; width: 100%; max-height: 85vh;
    display: flex; flex-direction: column; overflow: hidden;
    box-shadow: 0 20px 60px rgba(0,0,0,0.5);
  }
  .nfo-modal-head {
    padding: 14px 18px; border-bottom: 1px solid var(--border);
    display: flex; align-items: center; justify-content: space-between; gap: 12px;
  }
  .nfo-modal-title { font-size: 14px; font-weight: 600; color: var(--text); }
  .nfo-modal-sub { font-size: 11px; color: var(--text3); margin-top: 3px; word-break: break-all; }
  .nfo-modal-body {
    padding: 16px 18px; overflow-y: auto; flex: 1;
    font-family: ui-monospace, SFMono-Regular, Menlo, "Courier New", monospace;
    font-size: 11.5px; color: var(--text); line-height: 1.5;
    white-space: pre-wrap;
  }
  .nfo-modal-body :global(p) { margin: 0 0 2px; }
  .nfo-modal-body :global(a) { color: #60a5fa; text-decoration: none; word-break: break-all; }
  .nfo-modal-body :global(a:hover) { text-decoration: underline; }
  .nfo-modal-body :global(img) { max-width: 100%; height: auto; }

  .req-card {
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 12px;
    margin-bottom: 10px;
  }
  .req-status {
    padding: 2px 8px; border-radius: 10px; font-size: 10px;
    text-transform: uppercase; letter-spacing: 0.5px; font-weight: 600;
  }
  .req-status-pending { background: rgba(255, 214, 10, 0.15); color: var(--yellow); border: 1px solid rgba(255, 214, 10, 0.35); }
  .req-status-done    { background: rgba(126, 240, 192, 0.12); color: #7ef0c0; border: 1px solid rgba(126, 240, 192, 0.35); }
  .req-status-rejected{ background: rgba(255, 107, 107, 0.12); color: #ff6b6b; border: 1px solid rgba(255, 107, 107, 0.35); }

  .req-progress {
    margin-top: 10px;
    padding-top: 10px;
    border-top: 1px solid rgba(255,255,255,0.05);
  }
  .req-progress .progress-bar {
    height: 8px;
    background: rgba(255,255,255,0.06);
    border-radius: 4px;
    overflow: hidden;
  }
  .req-progress .progress-fill {
    height: 100%;
    background: linear-gradient(90deg, #00b4d8, #7ef0c0);
    transition: width 0.2s;
    box-shadow: 0 0 10px rgba(0,180,216,0.4);
  }
  .req-progress .progress-fill.done {
    background: #7ef0c0;
    box-shadow: 0 0 10px rgba(126,240,192,0.5);
  }

  .my-item {
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 12px;
    margin-bottom: 6px;
    display: flex;
    gap: 12px;
    align-items: center;
  }

  .modal-backdrop {
    position: fixed; inset: 0;
    background: rgba(0,0,0,0.6);
    backdrop-filter: blur(4px);
    display: flex; align-items: center; justify-content: center;
    z-index: 1000;
  }
  .modal-card {
    background: var(--paper, #1e1e24);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 20px 24px;
    max-width: 480px;
    width: 90%;
    max-height: 85vh;
    overflow-y: auto;
  }
  .modal-title { font-size: 15px; font-weight: 600; color: var(--text); margin-bottom: 4px; }
  .modal-hint { color: var(--text3); font-size: 12px; margin-bottom: 12px; }

  .stat-cell {
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 8px 10px;
  }
  .stat-k { font-size: 10px; color: var(--text3); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 4px; }
  .stat-v { font-size: 14px; font-weight: 600; color: var(--text); }

  .stat-bar {
    height: 8px;
    background: rgba(255,255,255,0.05);
    border-radius: 4px;
    overflow: hidden;
  }
  .stat-bar-fill {
    height: 100%;
    background: #00b4d8;
    transition: width 0.3s;
  }

  .api-log-row {
    padding: 8px 10px;
    border-bottom: 1px solid rgba(255,255,255,0.04);
    cursor: pointer;
    transition: background 0.1s;
  }
  .api-log-row:hover { background: rgba(255,255,255,0.02); }
  .api-log-err { background: rgba(255,107,107,0.04); }
  .api-log-method {
    display: inline-block;
    padding: 1px 8px;
    border-radius: 4px;
    font-weight: 700;
    font-size: 10px;
    letter-spacing: 0.5px;
    min-width: 50px;
    text-align: center;
  }
  .api-log-method-GET    { background: rgba(126,240,192,0.18); color: #7ef0c0; }
  .api-log-method-POST   { background: rgba(0,180,216,0.18);   color: #00b4d8; }
  .api-log-method-PUT    { background: rgba(255,214,10,0.18);  color: #ffd60a; }
  .api-log-method-PATCH  { background: rgba(255,214,10,0.18);  color: #ffd60a; }
  .api-log-method-DELETE { background: rgba(255,107,107,0.18); color: #ff6b6b; }

  .test-result {
    font-size: 12px; padding: 7px 11px; border-radius: 8px; margin-bottom: 12px;
    background: rgba(239, 68, 68, 0.08); color: #ff9585;
    border: 1px solid rgba(239, 68, 68, 0.25);
  }
  .test-result.ok {
    background: rgba(34, 197, 94, 0.08); color: #7ef0c0;
    border-color: rgba(34, 197, 94, 0.25);
  }

  .field { margin-bottom: 12px; }
  .field:last-child { margin-bottom: 0; }
  .field label { display: block; font-size: 11px; color: var(--text3); margin-bottom: 6px; text-transform: uppercase; letter-spacing: 0.6px; }
  .field-checkbox label { display: flex; align-items: center; gap: 8px; cursor: pointer; font-size: 13px; color: var(--text); margin-top: 8px; text-transform: none; letter-spacing: 0; }
  .field-hint { font-size: 11px; color: var(--text3); margin-top: 4px; }
  .pwd-row { display: flex; gap: 6px; align-items: stretch; }
  .pwd-row input { flex: 1; }
  .pwd-toggle {
    background: rgba(255,255,255,0.04); border: 1px solid var(--border);
    padding: 0 12px; font-size: 14px; border-radius: 8px;
    color: var(--text2); flex: none;
  }
  .pwd-toggle:hover { background: rgba(0, 180, 216, 0.08); border-color: rgba(0, 180, 216, 0.35); }
  .field-checkbox input[type=checkbox] { width: auto; }

  .fields-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px; }

  .btn-save {
    margin-top: 24px;
    color: #fff;
    background: var(--grad-primary);
    border: 1px solid rgba(0,0,0,0.25);
    padding: 11px 28px; font-size: 14px; font-weight: 600;
    letter-spacing: 0.2px;
    box-shadow:
      inset 0 1px 0 rgba(255,255,255,0.25),
      inset 0 -1px 0 rgba(0,0,0,0.2),
      0 1px 2px rgba(0,0,0,0.4),
      0 8px 24px -6px var(--accent-glow);
  }
  .btn-save:hover {
    background: var(--grad-primary-hover);
    filter: brightness(1.05);
    box-shadow:
      inset 0 1px 0 rgba(255,255,255,0.3),
      inset 0 -1px 0 rgba(0,0,0,0.2),
      0 2px 6px rgba(0,0,0,0.5),
      0 12px 32px -4px var(--accent-glow);
  }
  .btn-save:active { transform: translateY(1px); filter: brightness(0.95); }

  /* NZB live */
  .nzb-live { display: flex; flex-direction: column; gap: 14px; max-width: 600px; }
  .nzb-live-status {
    font-size: 14px; font-weight: 600; color: var(--blue-hot);
    padding: 11px 15px;
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px solid var(--border); border-radius: 10px;
  }
  .nzb-live-status.done { color: #7ef0c0; border-color: rgba(34, 197, 94, 0.3); background: rgba(34, 197, 94, 0.05); }
  .nzb-live-step { display: flex; align-items: center; gap: 10px; }
  .nzb-live-step > span:first-child { width: 60px; font-size: 11px; color: var(--text3); flex: none; text-transform: uppercase; letter-spacing: 0.6px; }
  .nzb-live-step .progress-bar { flex: 1; height: 8px; background: rgba(255,255,255,0.06); border-radius: 4px; overflow: hidden; }
  .nzb-live-step .progress-fill { height: 100%; background: var(--grad-primary); border-radius: 4px; transition: width 0.3s; box-shadow: 0 0 12px rgba(230, 57, 70, 0.3); }
  .nzb-live-step .pct { width: 40px; text-align: right; font-size: 12px; color: var(--text2); flex: none; font-variant-numeric: tabular-nums; }
  .nzb-live-meta { display: flex; gap: 16px; font-size: 12px; color: var(--text3); }
  .nzb-live-result { padding: 11px 15px; border-radius: 10px; font-size: 13px; font-weight: 500; }
  .nzb-live-result.ok {
    background: rgba(34, 197, 94, 0.08);
    border: 1px solid rgba(34, 197, 94, 0.25);
    color: #7ef0c0;
  }
  .nzb-live-result:not(.ok) {
    background: rgba(239, 68, 68, 0.08);
    border: 1px solid rgba(239, 68, 68, 0.25);
    color: #ff9585;
  }

  /* Journal */
  .log-toolbar { display: flex; justify-content: flex-end; margin-bottom: 10px; }
  .btn-log-clear {
    background: rgba(255,255,255,0.04);
    border: 1px solid var(--border); color: var(--text2);
    padding: 6px 14px; font-size: 11px; font-weight: 500;
    text-transform: uppercase; letter-spacing: 0.4px;
  }
  .btn-log-clear:hover { color: var(--red-hot); border-color: rgba(239, 68, 68, 0.3); }
  .log-list { display: flex; flex-direction: column; gap: 3px; font-size: 12px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  .log-line {
    display: flex; gap: 10px; align-items: baseline;
    padding: 5px 10px;
    background: rgba(255,255,255,0.02);
    border: 1px solid var(--border);
    border-radius: 6px;
  }
  .log-ts { color: var(--text3); flex: none; }
  .log-prefix { flex: none; font-weight: 700; padding: 2px 7px; border-radius: 4px; font-size: 10px; letter-spacing: 0.4px; }
  .log-prefix-ddl { background: rgba(0, 180, 216, 0.15); color: var(--blue-hot); }
  .log-prefix-nzb { background: rgba(247, 127, 0, 0.15); color: var(--orange); }
  .log-prefix-mi  { background: rgba(34, 197, 94, 0.15); color: #7ef0c0; }
  .log-prefix-meta{ background: rgba(255, 214, 10, 0.15); color: var(--yellow); }
  .log-prefix-tor { background: rgba(230, 57, 70, 0.18); color: var(--red-hot); }
  .log-msg { color: var(--text); word-break: break-all; }
  .log-prefix-chk { background: rgba(72, 202, 228, 0.18); color: var(--blue-hot); }
  .log-prefix-res { background: rgba(247, 127, 0, 0.15); color: var(--orange); }
  .log-prefix-watch { background: rgba(255, 214, 10, 0.18); color: var(--yellow); }
  .log-prefix-queue { background: rgba(255, 214, 10, 0.22); color: #ffe066; font-weight: 700; }
  .brand-version {
    font-size: 10px; color: var(--text3);
    font-family: ui-monospace, Menlo, monospace;
    letter-spacing: 0.6px; user-select: none;
  }
  .brand-author {
    font-size: 9px; color: var(--text3); opacity: 0.75;
    letter-spacing: 1.2px; text-transform: uppercase;
    margin-top: 2px; user-select: none;
  }
  .sidebar-footer { margin-top: auto; padding-top: 14px; }
  .btn-update {
    width: 100%;
    background: var(--grad-primary);
    color: #fff;
    border: 1px solid rgba(0,0,0,0.25);
    padding: 9px 12px;
    font-size: 11px; font-weight: 700;
    letter-spacing: 0.5px;
    border-radius: 8px;
    cursor: pointer;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.25), 0 6px 18px -6px var(--accent-glow);
    animation: pulse-update 2.4s ease-in-out infinite;
  }
  .btn-update:hover { filter: brightness(1.1); }

  .btn-check-update {
    width: 100%;
    background: rgba(255,255,255,0.04);
    color: var(--text3);
    border: 1px solid rgba(255,255,255,0.08);
    padding: 7px 10px;
    font-size: 11px; font-weight: 500;
    border-radius: 8px;
    cursor: pointer;
    display: flex; align-items: center; gap: 6px; justify-content: center;
  }
  .btn-check-update:hover { background: rgba(255,255,255,0.08); color: var(--text); }
  .btn-check-update:disabled { opacity: 0.5; cursor: not-allowed; }
  .sidebar.collapsed .btn-check-update { padding: 7px 4px; }
  @keyframes pulse-update {
    0%, 100% { box-shadow: inset 0 1px 0 rgba(255,255,255,0.25), 0 6px 18px -6px var(--accent-glow); }
    50%      { box-shadow: inset 0 1px 0 rgba(255,255,255,0.3),  0 10px 28px -4px var(--accent-glow); }
  }
  .log-prefix-fic { background: rgba(230, 57, 70, 0.18); color: var(--red-hot); }
  .fic-item {
    display: flex; align-items: center; gap: 10px;
    padding: 10px 0; border-bottom: 1px solid var(--border);
  }
  .fic-item:last-child { border-bottom: none; }
  .fic-main { flex: 1; display: flex; flex-direction: column; gap: 5px; min-width: 0; }
  .fic-name { font-size: 12px; color: var(--text); word-break: break-all; }
  .fic-meta { display: flex; flex-wrap: wrap; gap: 5px; }
  .fic-actions { display: flex; gap: 5px; flex: none; }

  /* Check Torrent */
  .check-tab { display: flex; flex-direction: column; gap: 14px; max-width: 1100px; }
  .check-header { display: flex; align-items: center; justify-content: space-between; }
  .check-actions { display: flex; gap: 8px; }
  .check-filters { display: flex; gap: 6px; }
  .filter-btn {
    display: inline-flex; align-items: center; gap: 7px;
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    color: var(--text2);
    padding: 7px 14px; font-size: 12px; font-weight: 500;
    border-radius: 8px;
    text-transform: uppercase; letter-spacing: 0.4px;
    transition: all 160ms ease;
  }
  .filter-btn:hover {
    background: rgba(0, 180, 216, 0.06);
    border-color: rgba(0, 180, 216, 0.25);
    color: var(--text);
  }
  .filter-btn.active {
    background: rgba(0, 180, 216, 0.12);
    border-color: rgba(0, 180, 216, 0.4);
    color: var(--text);
  }
  .filter-count {
    background: rgba(0,0,0,0.3); color: var(--text2);
    padding: 1px 7px; border-radius: 9999px; font-size: 10px;
    font-variant-numeric: tabular-nums;
  }
  .filter-btn.active .filter-count { background: rgba(0, 180, 216, 0.25); color: var(--blue-hot); }

  .chk-card {
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 14px 16px;
    display: flex; flex-direction: column; gap: 8px;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.05);
  }
  .chk-card.err { border-color: rgba(239, 68, 68, 0.3); background: rgba(239, 68, 68, 0.04); }
  .chk-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  .chk-name {
    font-weight: 600; color: var(--text); font-size: 13px;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .chk-badge {
    font-size: 11px; font-weight: 600;
    padding: 3px 10px; border-radius: 9999px;
    background: rgba(255,255,255,0.06); color: var(--text2);
    border: 1px solid var(--border);
    flex: none;
    text-transform: uppercase; letter-spacing: 0.4px;
  }
  .chk-badge.err { background: rgba(239, 68, 68, 0.12); color: #ff9585; border-color: rgba(239, 68, 68, 0.3); }
  .chk-badge.ok  { background: rgba(34, 197, 94, 0.12); color: #7ef0c0; border-color: rgba(34, 197, 94, 0.3); }
  .chk-msg { font-size: 11px; color: #ff9585; }
  .chk-file { font-size: 11px; color: var(--text3); font-family: ui-monospace, Menlo, monospace; }
  .chk-lihdl { font-size: 12px; color: #7ef0c0; }
  .chk-lihdl code { font-family: ui-monospace, Menlo, monospace; font-size: 11px; background: rgba(0,0,0,0.25); padding: 1px 6px; border-radius: 4px; }
  .chk-nomatch { font-size: 11px; color: var(--text3); font-style: italic; }
  .chk-btn { margin-top: 4px; padding: 9px 18px; font-size: 13px; }
  .chk-progress { display: flex; align-items: center; gap: 8px; font-size: 11px; color: var(--text2); }
  .chk-progress .progress-bar { flex: 1; height: 7px; background: rgba(255,255,255,0.06); border-radius: 4px; overflow: hidden; }

  /* Barre de progression inline pour les actions Torrent→seedbox et Reseed complet */
  .check-prog {
    margin-top: -4px; margin-bottom: 12px; padding: 8px 12px;
    background: rgba(108, 99, 255, 0.08); border: 1px solid rgba(108, 99, 255, 0.2);
    border-left: 3px solid #6c63ff; border-radius: 0 6px 6px 0;
    font-size: 11px;
  }
  .check-prog.done { background: rgba(126, 240, 192, 0.08); border-color: rgba(126, 240, 192, 0.35); border-left-color: #7ef0c0; }
  .check-prog.err  { background: rgba(239, 68, 68, 0.08); border-color: rgba(239, 68, 68, 0.35); border-left-color: #ef4444; }
  .check-prog-bar  { height: 6px; background: rgba(255,255,255,0.06); border-radius: 3px; overflow: hidden; margin-bottom: 4px; }
  .check-prog-fill { height: 100%; background: linear-gradient(90deg, #6c63ff, #a78bfa); transition: width 0.25s; }
  .check-prog.done .check-prog-fill { background: linear-gradient(90deg, #22c55e, #7ef0c0); }
  .check-prog.err  .check-prog-fill { background: #ef4444; }
  .check-prog-meta { display: flex; justify-content: space-between; gap: 10px; align-items: center; color: var(--text2); }
  .check-prog-msg  { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .check-prog-stats { font-family: ui-monospace, monospace; font-variant-numeric: tabular-nums; color: var(--text); font-size: 10.5px; white-space: nowrap; }
  .chk-progress .progress-fill { height: 100%; background: var(--grad-primary); transition: width 0.15s; }
  .chk-pct { width: 40px; text-align: right; font-variant-numeric: tabular-nums; }
  .chk-status { min-width: 180px; }
  .chk-done { font-size: 12px; color: #7ef0c0; font-weight: 600; }
</style>
