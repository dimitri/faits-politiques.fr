// Recherche locale : un index statique, chargé au premier usage. Aucun serveur,
// aucune requête sortante, aucun traceur. Sans JavaScript les déclencheurs
// restent masqués : on ne montre pas une commande morte.
(function(){
  var dlg=document.getElementById('palette'), q=document.getElementById('q'),
      res=document.getElementById('res'), idx=null, sel=0, cur=[];
  if(!dlg||!dlg.showModal) return;
  [].forEach.call(document.querySelectorAll('[data-rech]'),function(b){
    b.hidden=false; b.addEventListener('click',ouvrir);
  });
  function norm(s){return s.normalize('NFD').replace(/[̀-ͯ]/g,'').toLowerCase()}
  // La racine est lue sur le lien de la marque plutôt qu'écrite en dur : un
  // proxy de préversion réécrit les attributs href et les url() des feuilles de
  // style, jamais les chaînes d'un script ni le contenu d'un JSON. L'index est
  // donc appelé, et ses entrées résolues, à partir de cette racine — sans quoi
  // tout tombe en 404 dès que le site est monté sous un préfixe.
  var a0=document.querySelector('a.marque');
  var racine=(a0?a0.getAttribute('href'):'/').replace(/\/?$/,'/');
  function ouvrir(){
    dlg.showModal(); q.focus(); q.select();
    if(idx===null){ idx=[];
      fetch(racine+'recherche-index.json')
        .then(function(r){return r.json()})
        .then(function(d){ idx=d.map(function(e){e.k=norm(e.n+' '+(e.s||''));return e});
          rendre(q.value)})
        .catch(function(){ res.innerHTML='<p class="vide">L\'index n\'a pas pu être chargé.</p>' });
    }
  }
  function rendre(t){
    var n=norm(t.trim());
    if(!n){ cur=[]; res.innerHTML='<p class="vide">Tapez pour chercher parmi '+
      (idx?idx.length:0)+' entrées : députés, eurodéputés, candidats, partis, groupes.</p>'; return }
    cur=idx.filter(function(e){return e.k.indexOf(n)>=0})
      .sort(function(a,b){return a.k.indexOf(n)-b.k.indexOf(n)||a.n.length-b.n.length}).slice(0,40);
    if(!cur.length){ res.innerHTML='<p class="vide">Aucun résultat pour «&nbsp;'+
      t.replace(/[<>&]/g,'')+'&nbsp;». Les scrutins ne sont pas indexés.</p>'; return }
    sel=0;
    res.innerHTML=cur.map(function(e,i){
      return '<a href="'+racine+e.u+'" class="'+(i?'':'sel')+'"><span class="n">'+
        e.n.replace(/[<>&]/g,'')+'</span><span class="k">'+e.t+'</span></a>'}).join('');
  }
  q.addEventListener('input',function(){rendre(q.value)});
  dlg.addEventListener('keydown',function(e){
    if(e.key==='ArrowDown'||e.key==='ArrowUp'){
      e.preventDefault(); var a=res.querySelectorAll('a'); if(!a.length)return;
      a[sel].classList.remove('sel');
      sel=(sel+(e.key==='ArrowDown'?1:a.length-1))%a.length;
      a[sel].classList.add('sel'); a[sel].scrollIntoView({block:'nearest'});
    } else if(e.key==='Enter'){
      var a=res.querySelectorAll('a'); if(a.length){e.preventDefault(); a[sel].click()}
    }
  });
  document.addEventListener('keydown',function(e){
    if(e.key==='/'&&!/^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName)
       &&!dlg.open){ e.preventDefault(); ouvrir() }
  });
})();

// Sélecteur de thème : trois états. « auto » retire l'attribut et rend la main
// à prefers-color-scheme ; les deux autres le forcent. Rien ne quitte le
// navigateur, et le groupe reste masqué sans JavaScript — on ne montre pas une
// commande morte.
(function(){
  var grp=document.querySelector('.theme'); if(!grp) return;
  var btns=[].slice.call(grp.querySelectorAll('[data-theme-set]'));
  function lire(){ try{return localStorage.getItem('fp-theme')||'auto'}catch(e){return 'auto'} }
  function appliquer(v){
    if(v==='auto') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.setAttribute('data-theme',v);
    btns.forEach(function(b){b.setAttribute('aria-pressed',String(b.dataset.themeSet===v))});
  }
  grp.hidden=false;
  appliquer(lire());
  btns.forEach(function(b){b.addEventListener('click',function(){
    var v=b.dataset.themeSet;
    try{ v==='auto'?localStorage.removeItem('fp-theme'):localStorage.setItem('fp-theme',v) }catch(e){}
    appliquer(v);
  })});
})();

// Filtre local des relevés nominatifs : purement client, le site reste statique.
(function(){
  var f=document.querySelector('[data-filtre]'); if(!f) return;
  var tbl=document.getElementById(f.dataset.filtre); if(!tbl) return;
  var lignes=[].slice.call(tbl.tBodies[0].rows), champ=f.querySelector('input'),
      puces=[].slice.call(f.querySelectorAll('.puce')), grp='', cpt=f.querySelector('[data-compte]');
  function norm(s){return s.normalize('NFD').replace(/[̀-ͯ]/g,'').toLowerCase()}
  function passe(){
    var t=norm(champ?champ.value.trim():''), n=0;
    lignes.forEach(function(tr){
      var ok=(!t||norm(tr.cells[0].textContent).indexOf(t)>=0)&&(!grp||tr.dataset.g===grp);
      tr.hidden=!ok; if(ok)n++;
    });
    if(cpt) cpt.textContent=n;
  }
  if(champ) champ.addEventListener('input',passe);
  puces.forEach(function(b){b.addEventListener('click',function(){
    puces.forEach(function(o){o.setAttribute('aria-pressed','false')});
    b.setAttribute('aria-pressed','true'); grp=b.dataset.g||''; passe();
  })});
})();

// « Copier le lien » et « citer » : ce qui manquait au cas d'usage réel —
// contester une affirmation demande un lien qu'on colle, avec son empreinte.
[].forEach.call(document.querySelectorAll('[data-copier]'),function(b){
  b.hidden=false;
  b.addEventListener('click',function(){
    var src=b.dataset.copier, txt=src==='url'?location.href
      :(document.getElementById(src)||{textContent:''}).textContent.replace(/\s+/g,' ').trim();
    navigator.clipboard.writeText(txt).then(function(){
      var v=b.textContent; b.textContent='Copié'; setTimeout(function(){b.textContent=v},1600);
    });
  });
});
