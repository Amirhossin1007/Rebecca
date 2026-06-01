"""add node operations queue

Revision ID: 20_node_operations_queue
Revises: 19_telegram_backup_settings
Create Date: 2026-06-01 01:20:00.000000
"""
from __future__ import annotations

from alembic import op
import sqlalchemy as sa


revision = "20_node_operations_queue"
down_revision = "19_telegram_backup_settings"
branch_labels = None
depends_on = None


def _has_index(indexes: list[dict], name: str) -> bool:
    return any(index.get("name") == name for index in indexes)


def upgrade() -> None:
    bind = op.get_bind()
    inspector = sa.inspect(bind)
    tables = set(inspector.get_table_names())

    if "node_operations" not in tables:
        op.create_table(
            "node_operations",
            sa.Column("id", sa.Integer(), nullable=False),
            sa.Column("operation_type", sa.String(length=32), nullable=False),
            sa.Column("node_id", sa.Integer(), nullable=True),
            sa.Column("user_id", sa.Integer(), nullable=True),
            sa.Column("payload", sa.JSON(), nullable=False),
            sa.Column("status", sa.String(length=16), nullable=False, server_default="pending"),
            sa.Column("attempts", sa.Integer(), nullable=False, server_default="0"),
            sa.Column("last_error", sa.Text(), nullable=True),
            sa.Column("idempotency_key", sa.String(length=128), nullable=False),
            sa.Column("created_at", sa.DateTime(), nullable=False, server_default=sa.func.now()),
            sa.Column("updated_at", sa.DateTime(), nullable=False, server_default=sa.func.now()),
            sa.ForeignKeyConstraint(["node_id"], ["nodes.id"], ondelete="SET NULL"),
            sa.ForeignKeyConstraint(["user_id"], ["users.id"], ondelete="SET NULL"),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("idempotency_key", name="uq_node_operations_idempotency_key"),
        )

    inspector = sa.inspect(bind)
    indexes = inspector.get_indexes("node_operations")
    if not _has_index(indexes, "ix_node_operations_status_id"):
        op.create_index("ix_node_operations_status_id", "node_operations", ["status", "id"], unique=False)
    if not _has_index(indexes, "ix_node_operations_node_status_id"):
        op.create_index(
            "ix_node_operations_node_status_id",
            "node_operations",
            ["node_id", "status", "id"],
            unique=False,
        )
    if not _has_index(indexes, "ix_node_operations_user_id"):
        op.create_index("ix_node_operations_user_id", "node_operations", ["user_id"], unique=False)


def downgrade() -> None:
    bind = op.get_bind()
    inspector = sa.inspect(bind)
    tables = set(inspector.get_table_names())
    if "node_operations" not in tables:
        return

    indexes = {index["name"] for index in inspector.get_indexes("node_operations")}
    for index_name in (
        "ix_node_operations_user_id",
        "ix_node_operations_node_status_id",
        "ix_node_operations_status_id",
    ):
        if index_name in indexes:
            op.drop_index(index_name, table_name="node_operations")
    op.drop_table("node_operations")
