"""Track users disabled by admin state changes

Revision ID: 20_user_admin_disabled_at
Revises: 19_telegram_backup_settings
Create Date: 2026-06-04 01:00:00.000000
"""

from __future__ import annotations

from alembic import op
import sqlalchemy as sa


revision = "20_user_admin_disabled_at"
down_revision = "19_telegram_backup_settings"
branch_labels = None
depends_on = None


def _has_table(table_name: str) -> bool:
    return sa.inspect(op.get_bind()).has_table(table_name)


def _has_column(table_name: str, column_name: str) -> bool:
    if not _has_table(table_name):
        return False
    return column_name in {column["name"] for column in sa.inspect(op.get_bind()).get_columns(table_name)}


def upgrade() -> None:
    if not _has_table("users") or _has_column("users", "admin_disabled_at"):
        return

    with op.batch_alter_table("users") as batch:
        batch.add_column(sa.Column("admin_disabled_at", sa.DateTime(), nullable=True))


def downgrade() -> None:
    if not _has_table("users") or not _has_column("users", "admin_disabled_at"):
        return

    with op.batch_alter_table("users") as batch:
        batch.drop_column("admin_disabled_at")
