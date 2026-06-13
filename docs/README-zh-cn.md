<p align="center">
  <a href="https://github.com/rebeccapanel/Rebecca" target="_blank" rel="noopener noreferrer">
    <img width="160" height="160" src="../dashboard/src/assets/logo.svg" alt="Rebecca logo">
  </a>
</p>

<h1 align="center"/>Rebecca</h1>

<p align="center">
    Unified GUI Censorship Resistant Solution Powered by <a href="https://github.com/XTLS/Xray-core">Xray</a>
</p>

<br/>
<p align="center">
    <a href="#">
        <img src="https://img.shields.io/github/actions/workflow/status/rebeccapanel/Rebecca/build.yml?style=flat-square" />
    </a>
    <a href="https://hub.docker.com/r/rebeccapanel/rebecca" target="_blank">
        <img src="https://img.shields.io/docker/pulls/rebeccapanel/rebecca?style=flat-square&logo=docker" />
    </a>
    <a href="#">
        <img src="https://img.shields.io/github/license/rebeccapanel/Rebecca?style=flat-square" />
    </a>
    <a href="https://t.me/rebeccapanel_rebecca" target="_blank">
        <img src="https://img.shields.io/badge/telegram-channel-blue?style=flat-square&logo=telegram" />
    </a>
    <a href="#">
        <img src="https://img.shields.io/github/stars/rebeccapanel/Rebecca?style=social" />
    </a>
</p>

<p align="center">
	<a href="../README.md">
	English
	</a>
	/
	<a href="./README-fa.md">
	فارسی
	</a>
    /
  <a href="./README-zh-cn.md">
	简体中文
	</a>
   /
  <a href="./README-ru.md">
 Русский
 </a>
</p>

<p align="center">
  <a href="https://github.com/rebeccapanel/Rebecca" target="_blank" rel="noopener noreferrer" >
    <img src="https://github.com/rebeccapanel/Rebecca-docs/raw/master/screenshots/preview.png" alt="Rebecca screenshots" width="600" height="auto">
  </a>
</p>


## 目录
- [概览](#概览)
  - [为什么要使用 Rebecca?](#为什么要使用-rebecca)
    - [特性](#特性)
- [安装指南](#安装指南)
- [配置](#配置)
- [文档](#文档)
- [如何使用 API](#如何使用-api)
- [如何备份 Rebecca](#如何备份-rebecca)
- [Telegram bot](#telegram-bot)
- [捐赠](#捐赠)
- [许可](#许可)
- [贡献者](#贡献者)


# 概览

Rebecca 是一个代理管理工具，提供简单易用的用户界面，可管理数百个代理账户，由 [Xray-core](https://github.com/XTLS/Xray-core) 提供支持，使用 Go 后端和 Reactjs 构建。



## 为什么要使用 Rebecca?

Rebecca 是一个用户友好、功能丰富且可靠的工具。它让您可以为用户创建不同的代理，无需进行任何复杂的配置。通过其内置的 Web 界面，您可以监视、修改和限制用户。

### 特性

- 内置 **Web 界面**
- 完全支持 **REST API** 的后端
- 支持 **Vmess**、**VLESS**、**Trojan** 和 **Shadowsocks** 协议
- 单用户的**多协议**支持
- 单入站的**多用户**支持
- 单端口的**多入站**支持（使用 fallbacks）
- **流量**和**过期日期**限制
- 周期性的流量限制（例如每天、每周等）
- 兼容 **V2ray** 的**订阅链接**（例如 V2RayNG、SingBox、Nekoray 等）和 **Clash**
- 自动化的**分享链接**和**二维码**生成器
- 系统监控和**流量统计**
- 可自定义的 xray 配置
- **TLS** 支持
- 集成的 **Telegram Bot**
- **多管理员**支持（WIP）


# 安装指南
运行以下命令以使用 SQLite 数据库安装 Rebecca。

```bash
sudo bash -c "$(curl -sL https://raw.githubusercontent.com/rebeccapanel/Rebecca/master/scripts/rebecca/rebecca.sh)" @ install
```

运行以下命令以使用 MySQL 数据库安装 Rebecca。

```bash
sudo bash -c "$(curl -sL https://raw.githubusercontent.com/rebeccapanel/Rebecca/master/scripts/rebecca/rebecca.sh)" @ install --database mysql
```

运行以下命令以使用 MariaDB 数据库安装 Rebecca。
```bash
sudo bash -c "$(curl -sL https://raw.githubusercontent.com/rebeccapanel/Rebecca/master/scripts/rebecca/rebecca.sh)" @ install --database mariadb
```

Once the installation is complete:

- You will see the logs that you can stop watching them by closing the terminal or pressing `Ctrl+C`
- The Rebecca files will be located at `/opt/rebecca`
- The configuration file can be found at `/opt/rebecca/.env` (refer to [configurations](#configuration) section to see variables)
- The data files will be placed at `/usr/lib/rebecca`
- For security reasons, the Rebecca dashboard is not accessible via IP address. Therefore, you must [obtain SSL certificate](https://rebeccapanel.github.io/rebecca/en/examples/issue-ssl-certificate) and access your Rebecca dashboard by opening a web browser and navigating to `https://YOUR_DOMAIN:8000/dashboard/` (replace YOUR_DOMAIN with your actual domain)
- You can also use SSH port forwarding to access the Rebecca dashboard locally without a domain. Replace `user@serverip` with your actual SSH username and server IP and Run the command below:

```bash
ssh -L 8000:localhost:8000 user@serverip
```

Finally, you can enter the following link in your browser to access your Rebecca dashboard:

http://localhost:8000/dashboard/

You will lose access to the dashboard as soon as you close the SSH terminal. Therefore, this method is recommended only for testing purposes.

Next, you need to create a sudo admin for logging into the Rebecca dashboard by the following command

```bash
rebecca cli admin create --sudo
```

That's it! You can login to your dashboard using these credentials

To see the help message of the Rebecca script, run the following command

```bash
rebecca --help
```

If you are eager to run the project using the source code, check the section below
<details markdown="1">
<summary><h3>手动安装（高级）</h3></summary>

在您的机器上安装 xray

您可以使用 [Xray-install](https://github.com/XTLS/Xray-install) 脚本进行安装：

```bash
bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
```

克隆项目并构建 dashboard 和 Go 二进制文件：

```bash
git clone https://github.com/rebeccapanel/Rebecca.git
cd Rebecca
cd dashboard
npm ci
VITE_BASE_API=/api/ npm run build -- --outDir=build --assetsDir=statics
cp ./build/index.html ./build/404.html
cd ..
bash scripts/build_binary.sh
```

然后运行以下命令运行 Go 数据库迁移：

```bash
rebecca migrate up
```

现在开始配置：

复制 `.env.example` 文件，查看并使用文本编辑器（如`nano`）进行编辑。

您可能想要修改管理员凭据。

```bash
cp .env.example .env
nano .env
```

> 请查看[配置](#配置)部分以获取更多信息。

最终，使用以下命令启动应用程序：

```bash
./dist/rebecca-server
```

手动安装时，可以创建一个运行 Go 二进制文件的 systemd unit：

```ini
[Unit]
Description=Rebecca
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/rebecca
EnvironmentFile=/opt/rebecca/.env
ExecStart=/opt/rebecca/dist/rebecca-server
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

然后启用服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now rebecca
```

配合 nginx 使用：
```
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name  example.com;

    ssl_certificate      /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key  /etc/letsencrypt/live/example.com/privkey.pem;

    location ~* /(dashboard|statics|sub|api|docs|redoc|openapi.json) {
        proxy_pass http://0.0.0.0:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }

    # xray-core ws-path: /
    # client ws-path: /rebecca/me/2087
    #
    # 所有流量通过 443 端口进行代理，然后分发至真正的 xray 端口（2087、2088 等等）。
    # 路径中的 “/rebecca” 可以改为任意合法 URL 字符.
    #
    # /${path}/${username}/${xray-port}
    location ~* /rebecca/.+/(.+)$ {
        proxy_redirect off;
        proxy_pass http://127.0.0.1:$1/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```
或
```
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name  rebecca.example.com;

    ssl_certificate      /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key  /etc/letsencrypt/live/example.com/privkey.pem;

    location / {
        proxy_pass http://0.0.0.0:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

默认情况下，应用将在 `http://localhost:8000/dashboard` 上运行。您可以通过更改 `UVICORN_HOST` 和 `UVICORN_PORT` 环境变量来进行配置。
</details>

# 配置

> 您可以使用环境变量或将其放置在 `env` 或 `.env` 文件中来设置以下设置。

| 变量                                     | 描述                                                                                                                      |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| SUDO_USERNAME                            | Bootstrap 管理员用户名。                                                                                                  |
| SUDO_PASSWORD                            | Bootstrap 管理员密码。                                                                                                    |
| SQLALCHEMY_DATABASE_URL                  | 数据库 URL；Go runtime 仍保留这个 legacy 名称以兼容旧安装。                                                               |
| UVICORN_HOST                             | Go gateway 监听主机（默认：`0.0.0.0`）。                                                                                  |
| UVICORN_PORT                             | Go gateway 监听端口（默认：`8000`）。                                                                                     |
| UVICORN_SSL_CERTFILE                     | Go gateway TLS 证书路径。                                                                                                  |
| UVICORN_SSL_KEYFILE                      | Go gateway TLS 私钥路径。                                                                                                  |
| UVICORN_SSL_CA_TYPE                      | 安装脚本使用的 CA 类型：`public` 或 `private`。                                                                           |
| REBECCA_GATEWAY_ADDR                     | 完整 gateway 地址；会覆盖 `UVICORN_HOST`/`UVICORN_PORT`。                                                                 |
| REBECCA_NODE_OPERATIONS_POLL_INTERVAL    | Node operation 队列轮询间隔。                                                                                             |
| REBECCA_USER_LIFECYCLE_INTERVAL          | 用户 lifecycle 检查间隔。                                                                                                 |
| REBECCA_USER_USAGE_RESET_INTERVAL        | 用户周期性 usage reset 间隔。                                                                                             |
| USERS_AUTODELETE_DAYS                    | expired 用户多少天后自动删除；负数表示禁用。                                                                              |
| USER_AUTODELETE_INCLUDE_LIMITED_ACCOUNTS | auto-delete 是否包含 limited 用户。                                                                                       |
| JWT_ACCESS_TOKEN_EXPIRE_MINUTES          | JWT access token 过期时间（分钟）。                                                                                       |
| USERS_LIST_TIMEOUT_SECONDS               | 大型 user list 查询超时；`0` 表示禁用。                                                                                   |
| SUBSCRIPTION_READ_ONLY                   | 读取 subscription 时不更新 last-used metadata。                                                                           |
| XRAY_SUBSCRIPTION_URL_PREFIX             | CLI subscription-link helper 使用的 URL 前缀。                                                                            |
| XRAY_FALLBACKS_INBOUND_TAG               | 包含 fallback 的 inbound tag。                                                                                            |
| XRAY_EXCLUDE_INBOUND_TAGS                | 从 config/link 生成中排除的 inbound tags。                                                                                |
| REBECCA_APP_TEMPLATE_BASE                | 内置 templates 基础路径。                                                                                                 |
| REBECCA_CERT_BASE                        | 管理证书的基础路径。                                                                                                      |
| REBECCA_CONFIG_DIR                       | full backup 中包含的配置根目录。                                                                                          |
| GEO_TEMPLATES_INDEX_URL                  | 可选的 Geo template index URL。                                                                                           |
| REBECCA_WARP_API_BASE                    | Cloudflare WARP API base URL 覆盖值。                                                                                     |


# 文档
[Rebecca 文档](https://rebeccapanel.github.io/rebecca) 提供了所有必要的入门指南，支持三种语言：波斯语、英语和俄语。要全面覆盖项目的各个方面，这些文档需要大量的工作。我们欢迎并感谢您的贡献，以帮助我们改进文档。您可以在这个 [GitHub 仓库](https://github.com/rebeccapanel/Rebeccapanel.github.io) 中进行贡献。


# 如何使用 API
Rebecca 提供了 REST API，使开发人员能够以编程方式与 Rebecca 服务进行交互。


# 如何备份 Rebecca

定期备份 Rebecca 文件是预防系统故障或意外删除导致数据丢失的好习惯。以下是备份 Rebecca 的步骤：

1. 默认情况下，所有重要的 Rebecca 文件都保存在 `/var/lib/rebecca` ( Docker 版本)中。将整个 `/var/lib/rebecca` 目录复制到您选择的备份位置，比如外部硬盘或云存储。
2. 此外，请确保备份您的 `env` 文件，其中包含您的配置变量，以及您的 `Xray` 配置文件。

Rebecca 的备份服务会高效地压缩所有必要文件并将它们发送到您指定的 Telegram 机器人。它支持 SQLite、MySQL 和 MariaDB 数据库。其一个主要功能是自动化，允许您每小时安排一次备份。对于 Telegram 机器人的上传限制没有限制；如果文件超过限制，它会被拆分并以多个部分发送。此外，您可以在任何时间启动即时备份。

安装最新版 Rebecca 命令：
```bash
sudo bash -c "$(curl -sL https://raw.githubusercontent.com/rebeccapanel/Rebecca/master/scripts/rebecca/rebecca.sh)" @ install-script
```

设置备份服务：
```bash
rebecca backup-service
```

获取即时备份：
```bash
rebecca backup
```

按照这些步骤，您可以确保有备份所有 Rebecca 文件和数据，以及您的配置变量和 Xray 配置，以备将来恢复使用。请记得定期更新备份，以保持它们的最新性。


# Telegram bot

Rebecca 配备了一个集成的 Telegram bot，可以处理服务器管理、用户创建和删除，以及发送通知。通过几个简单的步骤，您可以轻松地启用这个机器人，并提供了一种方便的方式与 Rebecca 交互，而不需要每次都登录到服务器上。

启用 Telegram bot：

1. 将 `TELEGRAM_API_TOKEN` 设置为您的 bot API Token。
2. 将 `TELEGRAM_ADMIN_ID` 设置为您的 Telegram ID，您可以从 [@userinfobot](https://t.me/userinfobot) 中获取自己的 ID。


# 捐赠

如果您认为 Rebecca 有用，并想支持其发展，可以在以下加密网络之一进行捐赠：

- TRON network (TRC20): `TGftLESDAeRncE7yMAHrTUCsixuUwPc6qp`
- ETH, BNB, MATIC network (ERC20, BEP20): `0x413eb47C430a3eb0E4262f267C1AE020E0C7F84D`
- TON network: `UQDNpA3SlFMorlrCJJcqQjix93ijJfhAwIxnbTwZTLiHZ0Xa`


感谢您的支持！

# 许可

制作于 [Unknown!] 并在 [AGPL-3.0](./LICENSE) 下发布。

# 贡献者

我们热爱贡献者！如果您想做出贡献，请查看我们的[贡献指南](CONTRIBUTING.md)并随时提交拉取请求或打开问题。我们也欢迎您加入我们的 [Telegram](https://t.me/rebeccapanel_rebecca) 群组，以获得支持或贡献指导。

查看 [issues](https://github.com/rebeccapanel/Rebecca/issues) 以帮助改进这个项目。



<p align="center">
感谢所有为改善 Rebecca 做出贡献的贡献者们：
</p>
<p align="center">
<a href="https://github.com/rebeccapanel/Rebecca/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=rebeccapanel/Rebecca" />
</a>
</p>
<p align="center">
  Made with <a rel="noopener noreferrer" target="_blank" href="https://contrib.rocks">contrib.rocks</a>
</p>
