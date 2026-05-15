# Fratar OIDC Update Guide

Este arquivo documenta o procedimento para manter o fork do Mattermost atualizado com o patch de autenticacao `keycloak_oidc` / "Fratar OIDC".

Estado atual deste fork:
- branch de trabalho: `OIDC`
- base atual: `master`
- commits do patch:
  - `2118921dcc66efbe08ff7b2732a16868f81002be` - `feat: add Keycloak OIDC support`
  - `9c6fd8dfe2faa102cd09c3f366b6f68dd98d59ff` - `fix: update OAuth service regex to allow underscores and improve binary resolution in rsync script`

## 1. Setup inicial do remote oficial

Hoje este clone tem apenas o remote `origin`.

Se ainda nao existir um `upstream` apontando para o repositorio oficial, adicione uma vez:

```bash
cd /home/rodrigocoelho/repos/mattermost
git remote add upstream https://github.com/mattermost/mattermost.git
git remote -v
```

Se voce usar outro espelho oficial, ajuste a URL do `upstream`.

## 2. Fluxo recomendado para atualizar a base e reaplicar o patch

Este e o fluxo mais seguro quando o `master` oficial avancar.

### 2.1. Buscar tudo e validar arvore limpa

```bash
cd /home/rodrigocoelho/repos/mattermost
git status --short
git fetch origin
git fetch upstream
```

Se `git status --short` retornar algo, resolva antes de continuar.

### 2.2. Atualizar a base local

```bash
git switch master
git merge --ff-only upstream/master
git push origin master
```

Se o `merge --ff-only` falhar, significa que seu `master` local divergiu. Nesse caso, resolva a divergencia antes de seguir.

### 2.3. Fazer backup do branch OIDC atual

```bash
git switch OIDC
git branch OIDC-backup-$(date +%Y%m%d-%H%M)
```

### 2.4. Recriar o branch do patch em cima do master novo

Opcao recomendada: criar um branch novo e reaplicar os dois commits do patch.

```bash
git switch master
git switch -c OIDC-refresh-$(date +%Y%m%d)
git cherry-pick 2118921dcc66efbe08ff7b2732a16868f81002be
git cherry-pick 9c6fd8dfe2faa102cd09c3f366b6f68dd98d59ff
```

Se houver conflito:
- resolva os arquivos
- rode `git add <arquivo>`
- finalize com `git cherry-pick --continue`

Quando tudo estiver certo, substitua o branch principal do patch:

```bash
git branch -M OIDC
git push origin OIDC --force-with-lease
```

## 3. Arquivos mais sensiveis a conflito

Quando o upstream mudar bastante, revise com mais cuidado estes arquivos:

- `server/public/model/config.go`
- `server/public/model/keycloak_oidc.go`
- `server/public/model/user.go`
- `server/public/model/switch_request.go`
- `server/config/client.go`
- `server/config/diff.go`
- `server/config/utils.go`
- `server/cmd/mattermost/main.go`
- `server/channels/api4/user.go`
- `server/channels/app/oauth.go`
- `server/channels/app/oauth_keycloak_oidc.go`
- `server/channels/app/user.go`
- `server/channels/app/imports/import_validators.go`
- `server/channels/web/oauth.go`
- `server/go.mod`
- `webapp/platform/types/src/config.ts`
- `webapp/channels/src/components/login/login.tsx`
- `webapp/channels/src/components/signup/signup.tsx`
- `webapp/channels/src/components/user_settings/security/index.ts`
- `webapp/channels/src/components/user_settings/security/user_settings_security.tsx`
- `webapp/channels/src/components/user_settings/general/user_settings_general.tsx`
- `webapp/channels/src/components/admin_console/system_user_detail/system_user_detail.tsx`
- `webapp/channels/src/utils/constants.tsx`
- `server/scripts/rsync-build-cmd-to-remote.sh`

## 4. Compilacao

### 4.1. Build completo recomendado

Sempre que houver mudanca de backend e frontend:

```bash
cd /home/rodrigocoelho/repos/mattermost/server
make build-cmd
```

Esse e o caminho padrao para manter:
- `server/bin/mattermost`
- `server/bin/mmctl`
- `webapp/channels/dist`

### 4.2. Build rapido so do backend

Se voce alterou apenas backend:

```bash
cd /home/rodrigocoelho/repos/mattermost/server
go build ./cmd/mattermost
```

Observacao:
- `go build ./cmd/mattermost` gera `server/mattermost`
- o script de sync ja detecta esse binario automaticamente e usa o arquivo mais novo

## 5. Sincronizacao para o servidor

Destino atual do script:
- `root@192.168.1.2:/opt/mattermost.new/`

Dry-run:

```bash
cd /home/rodrigocoelho/repos/mattermost/server
./scripts/rsync-build-cmd-to-remote.sh --dry-run
```

Execucao real:

```bash
cd /home/rodrigocoelho/repos/mattermost/server
./scripts/rsync-build-cmd-to-remote.sh
```

O script sincroniza:
- binario `mattermost`
- `mmctl` se existir
- `client/`
- `fonts/`
- `i18n/`
- `templates/`
- `NOTICE.txt`
- `README.md`
- arquivos opcionais de licenca/manifest

O script nao toca em:
- `config/`
- `data/`
- `logs/`
- `plugins/`
- `prepackaged_plugins/`

## 6. Reinicio do servico

Depois da sincronizacao, reinicie o Mattermost no servidor usando o metodo que voce ja usa em producao.

Se o servico for systemd, o padrao costuma ser:

```bash
systemctl restart mattermost
systemctl status mattermost --no-pager
```

## 7. Validacoes depois do deploy

### 7.1. Validacao HTTP local no servidor

No servidor, a rota abaixo nao pode mais cair no SPA com `200 root.html`.

Ela deve seguir o fluxo OAuth:
- tipicamente `302 Found` para o Keycloak
- ou erro explicito de configuracao, se houver algo faltando

```bash
curl -I http://127.0.0.1:8065/oauth/keycloak_oidc/login
curl -I https://chat.fratar.com.br/oauth/keycloak_oidc/login
```

### 7.2. Validacao funcional web

Checklist:
- abrir `https://chat.fratar.com.br/login`
- confirmar botao `Fratar OIDC`
- clicar no botao
- confirmar redirecionamento para o Keycloak
- concluir login
- confirmar retorno para o Mattermost
- confirmar bind por email em usuario existente

### 7.3. Validacao funcional mobile

O app mobile oficial do Mattermost so exibe provedores SSO conhecidos. Para manter o backend usando
`keycloak_oidc` e ainda fazer o app mostrar a opcao, este fork publica o Fratar OIDC como alias
`openid` no client config quando `KeycloakOIDCSettings.Enable=true` e `OpenIdSettings.Enable=false`.

Validar o client config:

```bash
curl -s https://chat.fratar.com.br/api/v4/config/client | jq '.EnableSignUpWithOpenId, .EnableSignUpWithKeycloakOIDC, .OpenIdButtonText, .KeycloakOIDCButtonText'
```

Resultado esperado:

```text
"true"
"false"
"Fratar OIDC"
"Fratar OIDC"
```

Validar a rota que o app mobile oficial chama:

```bash
curl -sS -D - -o /dev/null 'https://chat.fratar.com.br/oauth/openid/mobile_login?redirect_to=mmauth://callback'
```

Resultado esperado:
- `302 Found`
- `Location:` apontando para o Keycloak
- `redirect_uri=` apontando para `https://chat.fratar.com.br/signup/keycloak_oidc/complete`

## 8. Verificacao do binario ativo

Se o frontend mostrar o botao, mas `/oauth/keycloak_oidc/login` continuar voltando para `/login?redirect_to=...`, quase sempre o problema e deploy do binario errado ou servico apontando para outra instalacao.

No servidor, confira:

```bash
readlink -f /proc/$(pgrep -x mattermost | head -1)/exe
systemctl cat mattermost | grep ExecStart
curl -I http://127.0.0.1:8065/oauth/keycloak_oidc/login
```

O binario ativo precisa ser o mesmo que foi sincronizado para `/opt/mattermost.new/bin/mattermost`.

## 9. Configuracao esperada

Bloco relevante do `config.json`:

```json
"GitLabSettings": {
    "Enable": false
},
"OpenIdSettings": {
    "Enable": false
},
"KeycloakOIDCSettings": {
    "Enable": true,
    "Secret": "SEU_CLIENT_SECRET_AQUI",
    "Id": "mattermost",
    "Scope": "openid profile email",
    "AuthEndpoint": "",
    "TokenEndpoint": "",
    "UserAPIEndpoint": "",
    "DiscoveryEndpoint": "https://auth.fratar.com.br/auth/realms/mattermost/.well-known/openid-configuration",
    "ButtonText": "Fratar OIDC",
    "ButtonColor": "#145DBF",
    "UsePreferredUsername": false
}
```

E no Keycloak:

```text
https://chat.fratar.com.br/signup/keycloak_oidc/complete
```

deve estar cadastrado como redirect URI valido.

Observacao para mobile:
- nao e necessario cadastrar `https://chat.fratar.com.br/signup/openid/complete` enquanto o alias estiver ativo
- o app chama `/oauth/openid/mobile_login`, mas o Mattermost redireciona o Keycloak para `/signup/keycloak_oidc/complete`
- os usuarios continuam autenticando com `auth_service = keycloak_oidc`

## 10. Fluxo rapido do dia a dia

Quando so quiser compilar e publicar rapidamente o branch `OIDC` atual:

```bash
cd /home/rodrigocoelho/repos/mattermost
git switch OIDC
git pull --ff-only origin OIDC

cd /home/rodrigocoelho/repos/mattermost/server
make build-cmd
./scripts/rsync-build-cmd-to-remote.sh
```

Depois, reinicie o servico no servidor e valide:

```bash
curl -I http://127.0.0.1:8065/oauth/keycloak_oidc/login
```

Se esse endpoint responder com `200 OK` e `root.html`, o binario ativo nao e o correto.
