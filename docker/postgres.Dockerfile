# L'image PostgreSQL de faits-politiques.fr.
#
# POURQUOI NOTRE PROPRE IMAGE. Il nous faut quatre extensions qu'aucune image
# publiée ne réunit :
#
#   postgis   les contours administratifs sont dessinés par la base elle-même,
#             en SVG, sans bibliothèque de carto ni serveur de tuiles ;
#   pgvector  la recherche par voisinage dans le corpus du Journal officiel ;
#   rum       la recherche de PHRASE et le classement par pertinence sans accès
#             au tas — mesuré : une phrase sur vecteur stocké coûte 15 ms là où
#             la même sur index fonctionnel en coûte 50 057, parce que GIN ne
#             range pas les positions des lexèmes. RUM les range (D-051) ;
#   unaccent  la configuration `fr` déaccentue avant de désuffixer, sans quoi
#             « Élysée » et « Elysee » sont deux mots (D-051).
#
# `postgis/postgis` n'a ni pgvector ni rum. `pgvector/pgvector` n'a pas PostGIS.
# Les assembler demande de construire, et construire demande de choisir une base.
#
# POURQUOI DEBIAN ET NON ALPINE. L'image précédente était Alpine, donc musl.
# Deux conséquences, la seconde beaucoup plus gênante que la première :
#
#   1. les paquets PGDG des extensions n'existent pas pour Alpine ; il faudrait
#      les compiler, donc les maintenir ;
#   2. musl et glibc ne classent pas les chaînes de la même façon. Les
#      collations diffèrent, donc l'ORDRE DES INDEX diffère. C'est pourquoi le
#      passage d'une image à l'autre se fait par pg_dump / pg_restore et jamais
#      par copie du volume : un répertoire de données recopié démarrerait, et
#      répondrait faux.
#
# La base est `debian:bookworm-slim` plus le dépôt PGDG, comme le laboratoire
# TAOP — c'est nativement multi-architecture, et les versions y sont à jour.
# L'outillage d'amorçage (docker-entrypoint.sh, gosu) est COPIÉ depuis l'image
# officielle plutôt que réécrit : l'initialisation de PGDATA, le traitement de
# docker-entrypoint-initdb.d et le passage des signaux sont exactement le genre
# de cas limites qu'il ne vaut pas la peine de résoudre à nouveau.
ARG POSTGRES_VERSION=17

FROM postgres:${POSTGRES_VERSION}-bookworm AS pg-entrypoint

FROM debian:bookworm-slim

ARG POSTGRES_VERSION=17
ARG POSTGIS_VERSION=3

# Mêmes identifiants d'utilisateur et de groupe que l'image officielle : un
# volume écrit par l'une doit rester lisible par l'autre.
# Voir https://github.com/docker-library/postgres/issues/274
RUN groupadd -r postgres --gid=999 && \
    useradd -r -g postgres --uid=999 --home-dir=/var/lib/postgresql --shell=/bin/bash postgres && \
    install --verbose --directory --owner postgres --group postgres --mode 1777 /var/lib/postgresql

# Les locales. `fr_FR.UTF-8` n'est pas un confort : les données de ce projet
# sont françaises, et un `ORDER BY nom` sans collation française range « Élysée »
# après « Zola ». Debian slim ne livre que C et C.UTF-8.
RUN apt-get update && apt-get install -y --no-install-recommends \
        curl ca-certificates gnupg locales && \
    { echo 'en_US.UTF-8 UTF-8'; echo 'fr_FR.UTF-8 UTF-8'; } >> /etc/locale.gen && \
    locale-gen && \
    rm -rf /var/lib/apt/lists/*
ENV LANG=en_US.utf8

# Le dépôt PGDG : c'est de là que viennent postgresql-$PG_MAJOR et, surtout, les
# extensions empaquetées pour cette version majeure précise. Le dépôt de Debian
# bookworm ne porte que PostgreSQL 15.
RUN install -d /usr/share/postgresql-common/pgdg && \
    curl -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc --fail \
        https://www.postgresql.org/media/keys/ACCC4CF8.asc && \
    echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt bookworm-pgdg main" \
        > /etc/apt/sources.list.d/pgdg.list

ENV PG_MAJOR=${POSTGRES_VERSION}
# Les binaires de la version EN TÊTE du PATH, pas en queue.
#
# En queue, `pg_isready`, `psql` ou `pg_dump` se résolvent en /usr/bin, c'est-à-
# dire vers le pg_wrapper de Debian, qui ne lance pas le binaire demandé : il
# cherche d'abord un « cluster » Debian dans /etc/postgresql et en adopte le
# port. Le premier démarrage de cette image l'a montré : le contrôle de santé
# interrogeait le port 5433 pendant que le serveur écoutait sur 5432, et le
# conteneur restait « unhealthy » en fonctionnant parfaitement.
ENV PATH=/usr/lib/postgresql/${PG_MAJOR}/bin:$PATH

# Et pas de cluster Debian du tout. Le paquet postgresql-$PG_MAJOR en crée un
# par défaut à l'installation (/etc/postgresql/$PG_MAJOR/main, port 5433 ici),
# qui ne sert à rien dans cette image — le serveur est lancé par
# docker-entrypoint.sh sur $PGDATA — et dont la seule présence égare les outils.
# C'est le réglage de l'image officielle, posé AVANT l'installation.
RUN mkdir -p /etc/postgresql-common/createcluster.d && \
    echo 'create_main_cluster = false' > /etc/postgresql-common/createcluster.d/00-pas-de-cluster.conf

# Le serveur et les extensions.
#
# postgresql-$PG_MAJOR livre déjà les contribs standard dont nous nous servons —
# pg_trgm, unaccent, btree_gist, btree_gin, pgcrypto, pg_stat_statements : leurs
# fichiers .control sont dans le paquet de base, il n'y a pas de paquet
# « contrib » séparé par version chez PGDG. Seules postgis, pgvector et rum
# demandent leur propre paquet.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        "postgresql-${PG_MAJOR}" \
        "postgresql-${PG_MAJOR}-postgis-${POSTGIS_VERSION}" \
        "postgresql-${PG_MAJOR}-postgis-${POSTGIS_VERSION}-scripts" \
        "postgresql-${PG_MAJOR}-pgvector" \
        "postgresql-${PG_MAJOR}-rum" \
    && rm -rf /var/lib/apt/lists/*

# LE DICTIONNAIRE FRANÇAIS.
#
# `french_stem`, le désuffixeur Snowball livré avec PostgreSQL, tronque les mots
# selon des règles mécaniques. Sur le vocabulaire du Journal officiel, il se
# trompe dans les deux sens — mesuré :
#
#   il CONFOND ce qui diffère
#     retraites -> retrait     et    retraité -> retrait
#     et « retrait » (d'un texte, d'une candidature) donne aussi « retrait ».
#     Chercher les retraites ramène les retraits.
#
#   il SÉPARE ce qui est identique
#     nomination -> nomin      mais   nommé -> nomm
#     ministre   -> ministr    mais   ministères -> minister
#     Chercher « nomination » ne trouve pas « a été nommé ».
#
# Un dictionnaire Ispell/Hunspell ne tronque pas : il ramène une forme fléchie à
# son LEMME par des règles morphologiques, en s'appuyant sur une liste de mots
# réels. « ministères » donne « ministère », et « retrait » reste « retrait ».
#
# Variante « toutes variantes » et non « classique » : le corpus va de 1861 à
# 2025 et contient donc les orthographes d'avant et d'après la réforme de 1990.
#
# Le dictionnaire est celui de Grammalecte (Olivier R.), sous licence MPL 2.0 —
# redistribuable, y compris dans une image.
#
# PostgreSQL veut les deux fichiers dans son propre répertoire, avec ses propres
# extensions : .dict et .affix. Il lit le format Hunspell, `FLAG long` compris.
RUN apt-get update &&     apt-get install -y --no-install-recommends hunspell-fr-comprehensive &&     cp /usr/share/hunspell/fr.dic "/usr/share/postgresql/${PG_MAJOR}/tsearch_data/fr_fr.dict" &&     cp /usr/share/hunspell/fr.aff "/usr/share/postgresql/${PG_MAJOR}/tsearch_data/fr_fr.affix" &&     rm -rf /var/lib/apt/lists/*

# Le contrôle qui évite de découvrir l'absence d'une extension au moment où on
# en a besoin : si un paquet a changé de nom chez PGDG, la construction échoue
# ici plutôt qu'à la première requête.
RUN set -eux; \
    for ext in postgis vector rum pg_trgm unaccent btree_gist btree_gin pgcrypto pg_stat_statements; do \
        test -f "/usr/share/postgresql/${PG_MAJOR}/extension/${ext}.control" \
            || { echo "extension manquante : ${ext}" >&2; exit 1; }; \
    done; \
    test ! -d /etc/postgresql/${PG_MAJOR} \
        || { echo "un cluster Debian a été créé malgré create_main_cluster = false" >&2; exit 1; }; \
    test "$(command -v pg_isready)" = "/usr/lib/postgresql/${PG_MAJOR}/bin/pg_isready" \
        || { echo "pg_isready passe par le pg_wrapper de Debian" >&2; exit 1; }; \
    for f in fr_fr.dict fr_fr.affix french.stop; do \
        test -s "/usr/share/postgresql/${PG_MAJOR}/tsearch_data/${f}" \
            || { echo "fichier de recherche manquant : ${f}" >&2; exit 1; }; \
    done

# Les extensions sont créées à l'initialisation d'un répertoire de données VIDE.
# Une restauration par pg_restore les recrée elle-même — le dump les porte — mais
# une base neuve doit pouvoir servir sans qu'on y pense.
COPY docker/initdb/01-extensions.sql /docker-entrypoint-initdb.d/01-extensions.sql

# initdb écrit listen_addresses='localhost' dans sa configuration d'exemple. Ce
# serveur est toujours joint depuis l'extérieur du conteneur : il doit écouter
# sur toutes les interfaces. C'est la correction que l'image officielle applique
# à sa propre configuration d'exemple.
RUN dpkg-divert --add --rename --divert "/usr/share/postgresql/postgresql.conf.sample.dpkg" "/usr/share/postgresql/${PG_MAJOR}/postgresql.conf.sample" && \
    cp -v /usr/share/postgresql/postgresql.conf.sample.dpkg /usr/share/postgresql/postgresql.conf.sample && \
    ln -sv ../postgresql.conf.sample "/usr/share/postgresql/${PG_MAJOR}/" && \
    sed -ri "s!^#?(listen_addresses)\s*=\s*\S+.*!\1 = '*'!" /usr/share/postgresql/postgresql.conf.sample && \
    grep -F "listen_addresses = '*'" /usr/share/postgresql/postgresql.conf.sample

# Même disposition de PGDATA et même outillage d'amorçage que l'image officielle.
RUN install --verbose --directory --owner postgres --group postgres --mode 3777 /var/run/postgresql
ENV PGDATA=/var/lib/postgresql/data
RUN install --verbose --directory --owner postgres --group postgres --mode 1777 "$PGDATA"
VOLUME /var/lib/postgresql/data

COPY --from=pg-entrypoint /usr/local/bin/docker-entrypoint.sh /usr/local/bin/docker-ensure-initdb.sh /usr/local/bin/
COPY --from=pg-entrypoint /usr/local/bin/gosu /usr/local/bin/gosu
RUN ln -sT docker-ensure-initdb.sh /usr/local/bin/docker-enforce-initdb.sh

ENTRYPOINT ["docker-entrypoint.sh"]
STOPSIGNAL SIGINT
EXPOSE 5432
CMD ["postgres"]
