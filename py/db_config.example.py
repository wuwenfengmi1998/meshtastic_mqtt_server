"""db_config 模板:复制为 db_config.py(已被 py/.gitignore 忽略)并按需修改。

口令一律从环境变量读取,严禁写进文件:
  export MESH_SOURCE_DB_PASSWORD='...'
  export MESH_TARGET_DB_PASSWORD='...'
"""

from __future__ import annotations

import os


def _require(name: str) -> str:
    value = os.environ.get(name, "")
    if not value:
        raise RuntimeError(f"环境变量 {name} 未设置。请先 export 口令再运行。")
    return value


SOURCE_CONFIG = {
    "host": os.environ.get("MESH_SOURCE_DB_HOST", "source-db.example.com"),
    "port": int(os.environ.get("MESH_SOURCE_DB_PORT", "3306")),
    "user": os.environ.get("MESH_SOURCE_DB_USER", "meshuser"),
    "password": _require("MESH_SOURCE_DB_PASSWORD"),
    "database": os.environ.get("MESH_SOURCE_DB_NAME", "meshtastic_db"),
    "charset": "utf8mb4",
}

TARGET_CONFIG = {
    "host": os.environ.get("MESH_TARGET_DB_HOST", "target-db.example.com"),
    "port": int(os.environ.get("MESH_TARGET_DB_PORT", "3306")),
    "user": os.environ.get("MESH_TARGET_DB_USER", "meshtastic_db"),
    "password": _require("MESH_TARGET_DB_PASSWORD"),
    "database": os.environ.get("MESH_TARGET_DB_NAME", "meshtastic_db"),
    "charset": "utf8mb4",
}
