# Gestion des credentials Basic Auth

Alpaca permet de stocker des credentials d'authentification Basic proxy dans le keychain système (macOS Keychain, GNOME Keyring, Windows Credential Manager). Chaque credential peut être associé à un proxy spécifique ou à un pattern glob.

## Sous-commandes

### Ajouter un credential

```sh
# Credential par défaut (utilisé quand aucun match spécifique)
alpaca credential add -u monlogin

# Credential pour un proxy spécifique
alpaca credential add proxy.corp.com -u admin

# Credential pour un pattern glob
alpaca credential add "*.corp.com" -u user2
```

Le mot de passe est toujours saisi via un prompt masqué dans le terminal. Il n'est jamais passé en argument de la ligne de commande.

### Supprimer un credential

```sh
# Supprimer le credential d'un proxy spécifique
alpaca credential remove proxy.corp.com

# Supprimer le credential par défaut
alpaca credential remove
```

### Lister les credentials

```sh
alpaca credential list
```

Affiche le pattern proxy et le login associé. Le mot de passe n'est jamais affiché.

Exemple de sortie :

```
PROXY                          LOGIN
proxy.corp.com                 admin
*.corp.com                     user2
(default)                      monlogin
```

## Résolution des credentials

Quand Alpaca reçoit une requête et doit s'authentifier auprès d'un proxy, il résout le credential à utiliser dans cet ordre :

1. **Match exact** - Le hostname du proxy correspond exactement à un account dans le keychain
2. **Match glob** - Le hostname du proxy correspond à un pattern glob dans le keychain (le pattern le plus spécifique gagne, c'est-à-dire le plus long)
3. **Flag `-b`** - La valeur passée via le flag `-b login:password` en ligne de commande
4. **Credential par défaut** - L'entrée du keychain avec le pattern `*`
5. **Aucun credential** - La requête est envoyée sans header `Proxy-Authorization`

### Exemple de résolution

Avec la configuration suivante :

```sh
alpaca credential add proxy.corp.com -u admin       # exact
alpaca credential add "*.corp.com" -u user2          # glob
alpaca credential add -u default-user                # default (*)
```

| Proxy              | Credential utilisé | Raison              |
|--------------------|--------------------|--------------------|
| `proxy.corp.com`   | `admin`            | Match exact         |
| `other.corp.com`   | `user2`            | Match glob          |
| `external.com`     | `default-user`     | Credential par défaut |

## Coexistence avec le flag `-b`

Le flag `-b` coexiste avec les credentials du keychain. La priorité est :

- Un match exact ou glob dans le keychain est **toujours prioritaire** sur `-b`
- Le flag `-b` est **prioritaire sur le credential par défaut** (`*`) du keychain
- Si ni `-b` ni le keychain ne fournissent de credential, aucune authentification n'est effectuée

Exemple :

```sh
alpaca credential add proxy.internal.com -u admin
alpaca -b fallback:pass -C http://pac-server/proxy.pac
```

- Requêtes via `proxy.internal.com` : utilisent le credential `admin` du keychain
- Requêtes via tout autre proxy : utilisent `fallback:pass` du flag `-b`

## Stockage

Les credentials sont stockés dans le keychain système de la plateforme :

| Plateforme      | Backend                              | Service         |
|-----------------|--------------------------------------|-----------------|
| macOS           | Keychain (via `go-keychain`)         | `alpaca-basic`  |
| Linux/GNOME     | Secret Service / Keyring (via `go-keyring`) | `alpaca-basic` |
| Windows         | Windows Credential Manager (via `go-keyring`) | `alpaca-basic` |

Chaque entrée utilise :
- **Service** : `alpaca-basic`
- **Account** : le pattern proxy (hostname, glob ou `*` pour le défaut)
- **Secret** : `login:password`

Ce service est distinct de celui utilisé pour les credentials NTLM (`alpaca`), il n'y a donc aucun conflit entre les deux.

## Cache

Les credentials résolus sont mis en cache en mémoire pour toute la durée du processus. Si vous ajoutez, modifiez ou supprimez un credential via `alpaca credential add/remove` pendant qu'Alpaca tourne, il faut redémarrer Alpaca pour que les changements soient pris en compte.

## Sécurité

- Les mots de passe ne sont jamais affichés par `credential list`
- Les mots de passe ne sont jamais passés en argument de commande, toujours saisis via prompt masqué
- Les mots de passe ne peuvent pas être stockés dans le fichier de configuration YAML
- Le stockage repose sur le keychain système, protégé par les mécanismes de sécurité de l'OS
