FROM python:3.13-slim

# 安装系统依赖 — python:3.13-slim 是精簡 Debian,curl / unzip 預設沒有
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    curl \
    unzip \
    && rm -rf /var/lib/apt/lists/*

# 设置工作目录
WORKDIR /app

# 复制依赖文件
COPY requirements.txt .

# fubon-neo 不是公开 PyPI 套件,而是富邦证券官方下載的 wheel(需签 API 服务
# 申请书后从 https://www.fbs.com.tw/TradeAPI/docs/download/download-sdk
# 取得)。从官方直接下載对应平台的 manylinux wheel 后 pip install(必须
# 先于 -r requirements.txt,因为后者会有个 noop 的 fubon-neo 行让 pip 报 404)。
# 官方 wheel URL 用 PEP 600 manylinux tag(aarch64 / x86_64),跟 Docker
# TARGETARCH(arm64 / amd64)命名不同,需用 case 转换。manylinux_2_17 wheel
# 透过 abi3 stable ABI 跟 Python 3.13 相容。升版只动 docker-compose 的
# FUBON_NEO_VERSION build arg。
ARG FUBON_NEO_VERSION=2.2.8
ARG TARGETARCH
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) WHEEL_ARCH=x86_64 ;; \
      arm64) WHEEL_ARCH=aarch64 ;; \
      *) echo "Unsupported TARGETARCH: ${TARGETARCH} (fubon-neo wheel only ships aarch64 + x86_64)"; exit 1 ;; \
    esac; \
    WHEEL_URL="https://www.fbs.com.tw/TradeAPI_SDK/fubon_binary/fubon_neo-${FUBON_NEO_VERSION}-cp37-abi3-manylinux_2_17_${WHEEL_ARCH}.manylinux2014_${WHEEL_ARCH}.zip"; \
    echo "Downloading fubon-neo wheel: TARGETARCH=${TARGETARCH} -> ${WHEEL_URL}"; \
    curl -fsSL -o /tmp/fubon_neo.zip "${WHEEL_URL}"; \
    unzip -j /tmp/fubon_neo.zip '*.whl' -d /tmp/fubon_wheel; \
    pip install --no-cache-dir /tmp/fubon_wheel/*.whl; \
    rm -rf /tmp/fubon_neo.zip /tmp/fubon_wheel

# 安装Python依赖
RUN pip install --no-cache-dir -r requirements.txt

# 创建非 root 用户
RUN useradd -m appuser

# 复制应用代码
COPY main.py .

# 设置文件所有权
RUN chown -R appuser:appuser /app

# 切换到非 root 用户
USER appuser

# 暴露端口
EXPOSE 8081

# 健康检查 (使用 127.0.0.1 對齊 Go client 硬編碼,避免雙棧 IPv6 歧義)
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8081/health')" || exit 1

# 启动命令
CMD ["python", "main.py"]
