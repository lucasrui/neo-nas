# Neo-NAS 文件备份工具

## 项目简介

Neo-NAS 是一个专门设计的文件备份工具，最初设计用于相机 SD 卡的自动备份，但同样适用于：

- 📸 相机 SD 卡自动备份
- 💾 U 盘文件备份
- 📁 任意文件夹的自动备份

### 特别说明

⚠️ 本工具与传统的同步工具有重要区别：

1. 这**不是**同步工具 - 源文件删除后，已备份的文件不会被删除
2. 这**不是**重复备份工具 - 同一文件只会备份一次，除非源文件发生变化
3. 采用单向备份策略 - 只从源位置备份到目标位置，不进行双向同步

## 快速开始

### 使用 Docker（推荐）

1. 拉取镜像：

```bash
docker pull ghcr.io/lucasrui/neo-nas:latest
```

2. 准备配置文件：
   创建 `config.json` 文件，示例如下：

```json
{
  "backup_configs": [
    {
      "source_dir": "/source/foo",
      "target_dir": "/target/bar"
    }
  ],
  "zip_configs": {
    "intervalSeconds": 3600, // 压缩间隔时间（秒）
    "items": [
      {
        "source": "/source/foo", // 源文件或文件夹路径
        "target": "/target/foo.zip" // 压缩文件存放路径
      }
    ]
  }
}
```

3. 运行容器：

```bash
docker run -v /path/to/config:/config -v /path/to/source:/source -v /path/to/target:/target ghcr.io/lucasrui/neo-nas:latest
```

### 使用 Docker Compose

1. 创建 `docker-compose.yml`：

```yaml
services:
  neo-nas:
    image: ghcr.io/lucasrui/neo-nas:latest
    volumes:
      - ./config:/config
      - /path/to/source:/source:ro,rslave
      - /path/to/target:/target
    environment:
      - BACKUP_CONFIG_DIR=/config
```

2. 启动服务：

```bash
docker-compose up -d
```

### 直接运行

`go run ./cmd/main.go`

## 配置说明

### 环境变量

`BACKUP_CONFIG_DIR` 是程序的核心环境变量，用于指定配置文件所在的目录路径。程序会在该目录下查找 `config.json` 文件，并在该目录下保存同步进度文件。

#### 环境变量设置方法

1. **Windows 系统**

   - 临时设置（当前会话有效）：

   ```powershell
   $env:BACKUP_CONFIG_DIR="C:\Users\xxx\config"
   ```

2. **Linux/macOS 系统**
   - 临时设置（当前会话有效）：
   ```bash
   export BACKUP_CONFIG_DIR="/home/user/config"
   ```

### 配置文件格式

配置文件 `config.json` 示例：

```json
{
  "backup_configs": [
    {
      "source_dir": "源文件夹路径",
      "target_dir": "目标文件夹路径",
      "target_user": "uid:gid" // 可选，指定目标文件的所有者
    }
  ],
  "zip_configs": {
    "intervalSeconds": 3600, // 压缩间隔时间（秒）
    "items": [
      {
        "source": "源文件或文件夹路径",
        "target": "压缩文件存放路径",
        "key": "加密密钥（可选）",
        "target_user": "uid:gid" // 可选，指定压缩文件的所有者
      }
    ]
  }
}
```

## 使用场景示例

1. **相机 SD 卡自动备份**

   - 将 SD 卡插入电脑后自动将照片备份到指定位置
   - 同一张照片不会重复备份
   - 即使删除 SD 卡中的照片，已备份的文件也会保留

2. **U 盘文件备份**

   - 插入 U 盘后自动备份重要文件
   - 支持增量备份，只备份新增或修改的文件

3. **工作文件备份**

   - 定期备份工作文件夹到外部存储
   - 保持备份文件的独立性，不受源文件删除影响

4. **定时压缩功能**
   - 定期压缩指定的文件或文件夹，生成压缩文件
   - 支持设置压缩间隔时间和密钥（可选）

## 注意事项

1. 本工具不会删除任何已备份的文件，即使源文件被删除
2. 文件备份采用一次性策略，除非文件内容变化，否则不会重复备份
3. 建议定期检查备份目录的存储空间
4. 首次运行时会进行完整备份，后续运行只会备份新增或修改的文件
