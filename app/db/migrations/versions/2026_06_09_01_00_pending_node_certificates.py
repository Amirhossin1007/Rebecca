"""add pending node certificates

Revision ID: 21_pending_node_certificates
Revises: 20_user_admin_disabled_at
Create Date: 2026-06-09 01:00:00.000000
"""

from __future__ import annotations

from alembic import op
import sqlalchemy as sa


revision = "21_pending_node_certificates"
down_revision = "20_user_admin_disabled_at"
branch_labels = None
depends_on = None


def _has_table(table_name: str) -> bool:
    return sa.inspect(op.get_bind()).has_table(table_name)


def _has_index(table_name: str, index_name: str) -> bool:
    if not _has_table(table_name):
        return False
    return any(index.get("name") == index_name for index in sa.inspect(op.get_bind()).get_indexes(table_name))


def upgrade() -> None:
    if not _has_table("pending_node_certificates"):
        op.create_table(
            "pending_node_certificates",
            sa.Column("id", sa.Integer(), nullable=False),
            sa.Column("token", sa.String(length=64), nullable=False),
            sa.Column("certificate", sa.Text(), nullable=False),
            sa.Column("certificate_key", sa.Text(), nullable=False),
            sa.Column("expires_at", sa.DateTime(), nullable=False),
            sa.Column("created_at", sa.DateTime(), nullable=False, server_default=sa.func.now()),
            sa.PrimaryKeyConstraint("id"),
            sa.UniqueConstraint("token", name="uq_pending_node_certificates_token"),
        )

    if not _has_index("pending_node_certificates", "ix_pending_node_certificates_expires_at"):
        op.create_index(
            "ix_pending_node_certificates_expires_at",
            "pending_node_certificates",
            ["expires_at"],
            unique=False,
        )


def downgrade() -> None:
    if not _has_table("pending_node_certificates"):
        return

    if _has_index("pending_node_certificates", "ix_pending_node_certificates_expires_at"):
        op.drop_index("ix_pending_node_certificates_expires_at", table_name="pending_node_certificates")
    op.drop_table("pending_node_certificates")
