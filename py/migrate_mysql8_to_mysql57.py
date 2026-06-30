#!/usr/bin/env python3
"""
MySQL 8 -> MySQL 5.7 数据库迁移脚本

依赖: pip install pymysql

使用方法:
    python py/migrate_mysql8_to_mysql57.py

功能:
    1. 从源 MySQL 8 读取表结构，修正 collation 后在目标 MySQL 5.7 建表
    2. 逐表批量拷贝数据
    3. 修正 auto-increment 值

注意:
    - 目标库应为空库（无同名表）
    - 运行前请确保源库和目标库都可连接
"""

from __future__ import annotations

import re
import sys
from typing import Any

import pymysql
import pymysql.cursors

# ============================================================
# 硬编码数据库配置
# ============================================================

SOURCE_CONFIG = {
    "host": "127.0.0.1",
    "port": 3306,
    "user": "root",
    "password": "PLEASE_CHANGE_ME",
    "database": "meshtastic",
    "charset": "utf8mb4",
}

TARGET_CONFIG = {
    "host": "127.0.0.1",
    "port": 3307,
    "user": "root",
    "password": "PLEASE_CHANGE_ME",
    "database": "meshtastic",
    "charset": "utf8mb4",
}

BATCH_SIZE = 5000

# ============================================================
# 工具函数
# ============================================================


def _conn(config: dict[str, Any]) -> pymysql.Connection:
    """创建数据库连接，使用 DictCursor 方便按列名访问"""
    return pymysql.connect(
        host=config["host"],
        port=config["port"],
        user=config["user"],
        password=config["password"],
        database=config["database"],
        charset=config["charset"],
        cursorclass=pymysql.cursors.DictCursor,
    )


def fix_collation(ddl: str) -> str:
    """将 MySQL 8 默认 collation 替换为 MySQL 5.7 兼容版本"""
    ddl = ddl.replace("utf8mb4_0900_ai_ci", "utf8mb4_general_ci")
    ddl = ddl.replace("utf8mb4_0900_as_ci", "utf8mb4_general_ci")
    return ddl


def get_all_tables(conn: pymysql.Connection) -> list[str]:
    with conn.cursor() as cur:
        cur.execute("SHOW TABLES")
        key = list(cur.description[0])[0]
        return [row[key] for row in cur.fetchall()]


def get_table_columns(conn: pymysql.Connection, table: str) -> list[str]:
    """返回表的所有列名（按顺序）"""
    with conn.cursor() as cur:
        cur.execute("SHOW COLUMNS FROM `%s`" % table)
        return [row["Field"] for row in cur.fetchall()]


def get_auto_increment(
    conn: pymysql.Connection, table: str
) -> int | None:
    """获取某张表当前的 AUTO_INCREMENT 值"""
    with conn.cursor() as cur:
        cur.execute(
            "SELECT AUTO_INCREMENT FROM information_schema.TABLES "
            "WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = %s",
            (table,),
        )
        row = cur.fetchone()
        if row and row["AUTO_INCREMENT"]:
            return int(row["AUTO_INCREMENT"])
    return None


def row_count(conn: pymysql.Connection, table: str) -> int:
    with conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) AS cnt FROM `%s`" % table)
        return cur.fetchone()["cnt"]


# ============================================================
# 主流程
# ============================================================


def migrate() -> int:
    src = _conn(SOURCE_CONFIG)
    tgt = _conn(TARGET_CONFIG)
    print(f"[连接] 源 MySQL 8 @ {SOURCE_CONFIG['host']}:{SOURCE_CONFIG['port']}")
    print(f"[连接] 目标 MySQL 5.7 @ {TARGET_CONFIG['host']}:{TARGET_CONFIG['port']}")
    print()

    # 1. 在目标库建库（如还不存在）
    try:
        with tgt.cursor() as cur:
            cur.execute(
                "CREATE DATABASE IF NOT EXISTS `%s` "
                "CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"
                % TARGET_CONFIG["database"]
            )
            cur.execute("USE `%s`" % TARGET_CONFIG["database"])
        tgt.select_db(TARGET_CONFIG["database"])
    except Exception as e:
        print(f"[错误] 创建目标数据库失败: {e}")
        return 1

    tables = get_all_tables(src)
    print(f"[发现] 源库共 {len(tables)} 张表: {', '.join(tables)}")
    print()

    # 2. 逐表建表（修正 collation）
    print("=" * 60)
    print("阶段 1: 在目标库创建表结构")
    print("=" * 60)
    for idx, table in enumerate(tables, 1):
        with src.cursor() as cur:
            cur.execute("SHOW CREATE TABLE `%s`" % table)
            row = cur.fetchone()
            ddl = row["Create Table"]
        ddl = fix_collation(ddl)

        try:
            with tgt.cursor() as cur:
                cur.execute(ddl)
            tgt.commit()
            print(f"  [{idx:2d}/{len(tables)}] OK `{table}`")
        except Exception as e:
            print(f"  [{idx:2d}/{len(tables)}] 错误 `{table}`: {e}")
            tgt.rollback()
            return 1

    print()

    # 3. 逐表拷贝数据
    print("=" * 60)
    print("阶段 2: 拷贝表数据")
    print("=" * 60)

    total_rows_copied = 0
    auto_increments: dict[str, int | None] = {}

    for idx, table in enumerate(tables, 1):
        columns = get_table_columns(src, table)
        if not columns:
            auto_increments[table] = None
            print(f"  [{idx:2d}/{len(tables)}] SKIP `{table}` (0 列)")
            continue

        col_quoted = ", ".join("`%s`" % c for c in columns)
        placeholders = ", ".join(["%s"] * len(columns))
        insert_sql = "INSERT INTO `%s` (%s) VALUES (%s)" % (
            table,
            col_quoted,
            placeholders,
        )

        table_count = 0
        with src.cursor() as read_cur:
            read_cur.execute("SELECT * FROM `%s`" % table)
            batch = read_cur.fetchmany(BATCH_SIZE)
            while batch:
                rows_values = [
                    [row.get(c) for c in columns] for row in batch
                ]
                try:
                    with tgt.cursor() as write_cur:
                        write_cur.executemany(insert_sql, rows_values)
                    tgt.commit()
                    table_count += len(rows_values)
                    print(
                        f"\r  [{idx:2d}/{len(tables)}] `{table}` -> {table_count} 行",
                        end="",
                        flush=True,
                    )
                except Exception as e:
                    print()
                    print(f"  [{idx:2d}/{len(tables)}] 错误 `{table}`: {e}")
                    tgt.rollback()
                    return 1
                batch = read_cur.fetchmany(BATCH_SIZE)

        # 修正 auto_increment
        ai = get_auto_increment(src, table)
        auto_increments[table] = ai
        if ai is not None:
            with tgt.cursor() as cur:
                cur.execute(
                    "ALTER TABLE `%s` AUTO_INCREMENT = %s" % (table, ai)
                )
            tgt.commit()

        total_rows_copied += table_count
        print(
            f"\r  [{idx:2d}/{len(tables)}] `{table}` -> {table_count} 行  [OK]"
        )

    # 4. 验证
    print()
    print("=" * 60)
    print("阶段 3: 验证行数")
    print("=" * 60)
    all_match = True
    src_total = 0
    tgt_total = 0
    for table in tables:
        s = row_count(src, table)
        t = row_count(tgt, table)
        src_total += s
        tgt_total += t
        status = "OK" if s == t else "不匹配!"
        if s != t:
            all_match = False
        print(f"  `{table}`: 源={s}  目标={t}  [{status}]")

    print()
    print(f"  总计: 源={src_total}  目标={tgt_total}")

    src.close()
    tgt.close()

    if all_match:
        print()
        print("迁移完成，所有表行数一致。")
        return 0
    else:
        print()
        print("警告: 部分表行数不一致，请检查。")
        return 1


if __name__ == "__main__":
    sys.exit(migrate())
