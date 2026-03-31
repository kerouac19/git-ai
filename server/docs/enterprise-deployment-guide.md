# Git-AI 企业版部署指南

## 1. 部署概述

补充文档:

- [私有化部署拆解: `git-ai login` / `git-ai dash`](/Users/hg/git/git-ai/.docs/private-deployment-login-dash-implementation-plan.md)
- [Server 实现审查记录（2026-03-27）](/Users/hg/git/git-ai/.docs/server-implementation-gap-review-2026-03-27.md)

### 1.1 架构介绍
Git-AI 企业版是一个分布式系统，包含多个组件协同工作:
- **API服务器**: 处理HTTP请求和业务逻辑 (NestJS/Node.js)
- **数据库**: PostgreSQL - 存储用户、配置和事务数据  
- **缓存**: Redis - 提升频繁数据查询性能
- **CAS存储**: 内容寻址存储 - 持久化AI贡献的代码内容
- **逆向代理**: Nginx (可选) - HTTP流量管理、SSL终止

### 1.2 系统组件说明
```
┌─────────────────┐    ┌──────────────┐
│ Git-AI Clients  │◄──►│  Load        │
│ (IDE, CLI, etc) │    │  Balancer    │
└─────────────────┘    └──────────────┘
                                  │
                    ┌─────────────▼─────────────┐
                    │        Reverse Proxy      │
                    │        (Nginx/HAProxy)    │
                    └─────────────┬─────────────┘
                                  │
                    ┌─────────────▼─────────────┐
┌─────────────────┐ │     API Servers         │┌──────────────────┐
│   CAS Storage   │ │    (Multiple Nodes)     ││   Monitoring    │
│  (Persistent   │◄├─                        ─┤│   (Prometheus,  │
│   Storage)     │ │                         ││   Grafana, etc) │
└─────────────────┘ └─────────────────────────┘└──────────────────┘
                              │
                    ┌─────────▼─────────┐
                    │     Cache         │
                    │    (Redis)        │
                    └─────────┬─────────┘
                              │
                    ┌─────────▼─────────┐
                    │    Database       │
                    │   (PostgreSQL)    │
                    └───────────────────┘
```

### 1.3 部署规模选项

| 规模 | 用途 | 推荐配置 | 并发用户数 | 负载说明 |
|------|------|----------|------------|----------|
| 小型 | 开发团队/测试 | 4核, 8GB RAM | ≤ 20 | 单节点部署 |
| 中型 | 部门级部署 | 8核, 32GB RAM | 20-100 | 基础HA配置 | 
| 企业级 | 全公司部署 | 16+核, 64GB+ RAM | 100+ | 多活高可用 |

## 2. 先决条件

### 2.1 系统要求
- **操作系统**: Linux (Ubuntu 20.04+, CentOS/RHEL 8+, Debian 11+), macOS, Windows 10/11 (WSL2)
- **Docker**: CE v20.10+ 或 Podman v4.0+
- **Docker Compose**: v2.0+
- **内存**: 至少2GB，建议4GB以上用于生产环境
- **存储**: 根据代码库大小 (建议预留100GB+)，特别是CAS存储区域

### 2.2 依赖项
- Docker Engine
- Docker Compose Plugin 或独立Compose二进制文件
- Git v2.0+
- curl 或 wget (用于下载和健康检查)
- OpenSSL (用于SSL/TLS证书)

### 2.3 权限要求
- **Docker权限**: 确保运行部署的用户在docker组中
- **网络权限**: 访问外网以拉取Docker镜像（可在Air-gapped环境中预先加载镜像）
- **文件权限**: 写入配置、日志、CAS数据目录

## 3. 逐步安装指南

### 3.1 Docker/Docker Compose 方式 (推荐)

#### 第一步: 预备操作
```bash
# 1. 创建部署目录结构
mkdir -p git-ai-deployment/{config,data,logs,cas-storage}
cd git-ai-deployment

# 2. 下载部署文件 (示例: docker-compose.yml 和配置模板)
wget https://github.com/your-org/git-ai/releases/latest/download/docker-compose.yml
wget https://github.com/your-org/git-ai/releases/latest/download/.env.example
cp .env.example .env
```

#### 第二步: 环境配置
```bash
# 编辑环境配置
vim .env
```

**关键环境变量设置示例:**
```bash
# .env
APP_NAME=GitAI Enterprise
NODE_ENV=production
PORT=3000

# 数据库配置
POSTGRES_HOST=gitai-db
POSTGRES_PORT=5432
POSTGRES_DB=gitaidb
POSTGRES_USER=gitai_user
POSTGRES_PASSWORD=your_secure_password

# Redis配置
REDIS_HOST=gitai-redis
REDIS_PORT=6379
REDIS_PASSWORD=your_redis_password

# CAS存储路径
CAS_STORAGE_PATH=/data/cas-storage

# 认证配置
JWT_SECRET=very_long_secure_random_string_here
OAUTH_CLIENT_ID=gitai_client_app
OAUTH_CLIENT_SECRET=very_long_client_secret

# 网络和安全
TRUST_PROXY=true
CORS_ORIGIN=https://your-domain.com
HTTPS_REDIRECT=false  # 设为true时需配置reverse proxy
```

#### 第三步: 启动服务
```bash
# 预先拉取镜像 (确保网络连接良好)
docker pull your-registry.git-ai.com/server:latest
docker pull your-registry.git-ai.com/db:latest  
docker pull your-registry.git-ai.com/redis:latest

# 验证 docker-compose 文件语法
docker compose config

# 启动服务
docker compose up -d

# 验证服务状态
docker compose ps

# 查看初始化日志
docker compose logs --tail 50
```

#### 第四步: 健康检查
```bash
# 等待服务初始化 (约2-3分钟)
sleep 180

# 检查端点是否可用
curl -I http://localhost:3000/health

# 检查API状态
curl http://localhost:3000/api/status

# 查看详细容器日志
docker logs gitai-server-1 --tail 20
```

### 3.2 Kubernetes 方式 (企业级部署)

#### 部署Kubernetes清单 (简化的示例)
```yaml
# k8s/git-ai-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gitai-server
spec:
  replicas: 2  # 为了高可用性
  selector:
    matchLabels:
      app: gitai-server
  template:
    metadata:
      labels:
        app: gitai-server
    spec:
      containers:
        - name: server
          image: your-registry.git-ai.com/server:latest
          ports:
            - containerPort: 3000
          env:
            - name: DB_HOST
              value: "gitai-postgres-service"
            - name: REDIS_HOST  
              value: "gitai-redis-service"
            - name: JWT_SECRET
              valueFrom:
                secretKeyRef:
                  name: gitai-secrets
                  key: jwt-secret
          volumeMounts:
            - name: cas-storage
              mountPath: /data/cas
      volumes:
        - name: cas-storage
          persistentVolumeClaim:
            claimName: gitai-cas-storage-pvc

---
apiVersion: v1
kind: Service
metadata:
  name: gitai-server-service
spec:
  selector:
    app: gitai-server
  ports:
    - protocol: TCP
      port: 80
      targetPort: 3000
  type: ClusterIP
```

#### 部署到K8s
```bash
# 部署到命名空间
kubectl create namespace gitai
kubectl apply -f k8s/secrets.yaml  # 先部署密钥
kubectl apply -f k8s/postgres.yaml # 数据库(PVC, Deployment, Service)  
kubectl apply -f k8s/redis.yaml    # Redis服务
kubectl apply -f k8s/git-ai-deployment.yaml # 应用部署

# 检查部署状态
kubectl get pods -n gitai
kubectl get services -n gitai

# 检查Pod日志
kubectl logs -n gitai deployment/gitai-server -f
```

## 4. 环境配置详解

### 4.1 环境变量说明
| 变量名 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `NODE_ENV` | String | development | 环境模式: development/production/testing |
| `PORT` | Number | 3000 | API服务器端口 |
| `DB_HOST` | String | localhost | 数据库主机地址 |
| `DB_USERNAME`, `DB_PASSWORD` | String | - | 数据库认证凭据 |
| `REDIS_URL` | String | redis://localhost:6379 | Redis连接URL |
| `JWT_SECRET` | String | - | JWT令牌签名密钥（至少64位随机字符串） |
| `GITAI_CAS_PATH` | String | ./cas-storage | CAS存储绝对路径 |
| `REQUEST_TIMEOUT` | Number | 30000 | 默认请求超时时间(毫秒) |

### 4.2 数据库配置
#### 环境特定设置
```bash
# 生产数据库配置示例
DATABASE_CONNECTION_POOL_SIZE=50
DATABASE_TIMEOUT=5000
DATABASE_IDLE_TIMEOUT=300000

# 数据库迁移配置
DATABASE_RUN_MIGRATIONS_ON_STARTUP=true
DATABASE_SYNCHRONIZE=false  # 生产环境禁止自动同步Schema
```

#### 适配不同数据库 (PostgreSQL示例)
```bash
DB_TYPE=postgres
DATABASE_URL="postgresql://username:password@host:port/database?sslmode=require"
```

### 4.3 网络设置
```bash
# CORS设置
CORS_ALLOWED_ORIGINS="https://your-git-domain.com,https://your-other-domain.com"
CORS_ALLOW_CREDENTIALS=true

# 代理配置
HTTP_PROXY="http://proxy.yourcompany.com:8080"
HTTPS_PROXY="https://proxy.yourcompany.com:8080"
NO_PROXY="localhost,127.0.0.1,.internal.company.com"

# 服务器IP绑定
HOST=0.0.0.0  # 绑定到所有接口，生产环境仅在必要时使用
```

## 5. 高可用性配置

### 5.1 负载均衡设置

#### 使用NGINX作为负载均衡器
```nginx
# nginx.conf
upstream gitai_backend {
    least_conn;  # 优先连接活跃连接数最少的服务器
    server gitai-app-01:3000 weight=1 max_fails=2 fail_timeout=10s;
    server gitai-app-02:3000 weight=1 max_fails=2 fail_timeout=10s;
    keepalive 32;  # 保持连接以提升性能
}

server {
    listen 80; 
    server_name gitai.yourcompany.com;

    location / {
        proxy_pass http://gitai_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # 超时设置
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        proxy_buffer_size 4k;
        proxy_buffers 4 32k;
        proxy_busy_buffers_size 64k;
    }
}
```

### 5.2 多实例部署
```yaml
# docker-compose-ha.yml
version: '3.8'
services:
  gitai-server-1:
    image: your-registry.git-ai.com/server:latest
    # 健康检查和资源限制
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 2G
        reservations:
          cpus: '0.5'
          memory: 1G

  gitai-server-2:
    image: your-registry.git-ai.com/server:latest
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 2G
        reservations:
          cpus: '0.5'
          memory: 1G
            
  # 共享数据库和Redis
  pg:
    # ... 数据库配置相同
  redis:
    # ... Redis配置相同
```

### 5.3 故障转移配置
- **数据库集群**: 使用PgBouncer + Patroni + etcd 实现PostgreSQL HA
- **Redis集群**: 哨兵模式或集群模式
- **应用层**: 健康检查 + 自动恢复 + 负载均衡器失效转移

## 6. 监控和日志集成

### 6.1 应用日志配置
```javascript
// 伪代码，实际在日志配置文件中或代码中指定
const winston = require('winston');

winston.createLogger({
  level: 'info',
  format: winston.format.combine(
    winston.format.timestamp(),
    winston.format.errors().stackTraceFormat(),
    winston.format.splat(),
    winston.format.json()
  ),
  defaultMeta: { service: 'git-ai-server' },
  transports: [
    new winston.transports.File({ filename: 'error.log', level: 'error' }),
    new winston.transports.File({ filename: 'combined.log' }),
    new winston.transports.Console({
      format: winston.format.simple()
    })
  ]
});
```

### 6.2 Prometheus集成
#### 在 .env 中启用指标导出
```bash
ENABLE_METRICS=true
METRICS_PORT=3001
METRICS_ROUTE=/metrics
```

#### Prometheus抓取配置
```yaml
# prometheus.yml 
scrape_configs:
  - job_name: 'gitai'
    static_configs:
      - targets: ['gitai-server-1:3001', 'gitai-server-2:3001']
```

### 6.3 企业监控系统集成
- **ELK Stack**: 使用Filebeat采集日志，Elasticsearch存储和Kibana可视化
- **DataDog/NewRelic**: 使用APM代理收集应用性能数据
- **Grafana Loki**: 集中日志聚合与查询

## 7. 维护和日常管理

### 7.1 备份策略
```bash
#!/bin/bash
# backup-script.sh - 定期备份脚本
DATE=$(date +%Y%m%d_%H%M)
BACKUP_DIR="/backup/gitai/${DATE}"

mkdir -p $BACKUP_DIR

# 1. PostgreSQL备份 (需要PGPASSWORD环境变量)
pg_dump -h db-host.domain.com -U gitai_user gitaidb > $BACKUP_DIR/db_backup.sql

# 2. CAS存储备份  
rsync -av --progress /data/cas-storage/ $BACKUP_DIR/cas_data/

# 3. 配置和日志备份
rsync -av --exclude "*.log" /app/config/ $BACKUP_DIR/config/

# 4. 压缩和清理旧备份
tar -czf $BACKUP_DIR.tar.gz $BACKUP_DIR
find /backup/gitai -name "*.tar.gz" -mtime +7 -delete
```

### 7.2 升级流程
```bash
# 1. 确认当前版本
docker compose images
curl -s http://localhost:3000/api/version

# 2. 做好升级前备份
./backup-script.sh

# 3. 拉取新版本镜像
docker compose pull

# 4. 测试更新（滚动更新）
# 在Kubernetes中，只需改变Deployment的image
docker compose up -d --no-deps --force-recreate gitai-server

# 5. 验证升级成功
curl -s http://localhost:3000/health

# 6. 监控系统日志检查是否有错误
docker compose logs -f --tail=50 gitai-server
```

### 7.3 日常维护任务
- **容量检查**: 定期检查CAS存储增长情况
- **性能监控**: 关注API响应时间和资源使用
- **日志清理**: 清除过旧的日志文件（保留策略）
- **安全更新**: 定期更新基础镜像
- **服务重启**: 策略性重启清理可能的内存泄漏（比如每周重启）

## 8. 部署配置文件示例

### 8.1 Standard Docker Compose (docker-compose.yaml)
```yaml
version: '3.8'
services:
  # Git-AI API Server - 主要应用
  api-server:
    image: your-registry.git-ai.com/server:latest
    container_name: gitai-api-server
    restart: unless-stopped
    ports:
      - "3000:3000"
    environment:
      - NODE_ENV=production
      - PORT=3000
      - DATABASE_URL=postgresql://gitai_user:${DB_PASSWORD}@db:5432/gitaidb
      - REDIS_URL=redis://redis:6379
      - JWT_SECRET=${JWT_SECRET}
      - CORS_ORIGIN=${CORS_ORIGIN}
    volumes:
      - ./cas-storage:/data/cas:rw
      - ./config:/app/config:ro
      - ./logs:/app/logs
    depends_on:
      - db
      - redis
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 60s

  # PostgreSQL Database
  db:
    image: postgres:15-alpine
    container_name: gitai-db
    restart: unless-stopped
    environment:
      - POSTGRES_DB=gitaidb
      - POSTGRES_USER=gitai_user
      - POSTGRES_PASSWORD=${DB_PASSWORD}
    volumes: 
      - gitai_postgres_data:/var/lib/postgresql/data
      - ./init-scripts:/docker-entrypoint-initdb.d
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U gitai_user -d gitaidb"]
      interval: 10s
      timeout: 5s
      retries: 5

  # Redis Cache
  redis:
    image: redis:7-alpine
    container_name: gitai-redis
    restart: unless-stopped
    command: redis-server --requirepass ${REDIS_PASSWORD}
    volumes:
      - gitai_redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  gitai_postgres_data:
  gitai_redis_data:
```

### 8.2 生产环境推荐(.env)
```bash
DB_PASSWORD=your_strong_prod_db_password
REDIS_PASSWORD=your_strong_redis_password
JWT_SECRET=at_least_64_characters_of_completely_random_alpha_numeric_string!
CORS_ORIGIN=https://your-git-server.domain.com,https://your-corp-domain.com
TRUST_PROXY=true
NODE_ENV=production
LOG_LEVEL=info
HTTPS_ONLY=true
SSL_CERT_PATH=/etc/ssl/certs/gitai-cert.pem
SSL_KEY_PATH=/etc/ssl/private/gitai-key.pem
```

这个部署指南为企业部署Git-AI提供了完整的端到端指导，包含各种部署选项、配置示例和维护建议。
