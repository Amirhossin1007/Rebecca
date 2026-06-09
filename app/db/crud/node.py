"""
Functions for managing proxy hosts, users, user templates, nodes, and administrative tasks.
"""

from typing import List, Optional, Union

from sqlalchemy.orm import Session
from app.db.models import Node
from app.models.node import NodeStatus


def get_node(db: Session, name: Optional[str] = None, node_id: Optional[int] = None) -> Optional[Node]:
    """Retrieves a node by its name or ID."""
    query = db.query(Node)
    if node_id is not None:
        return query.filter(Node.id == node_id).first()
    elif name:
        return query.filter(Node.name == name).first()
    return None


def get_node_by_id(db: Session, node_id: int) -> Optional[Node]:
    """Wrapper for backward compatibility."""
    return get_node(db, node_id=node_id)


def get_nodes(
    db: Session, status: Optional[Union[NodeStatus, list]] = None, enabled: bool = None, include_master: bool = False
) -> List[Node]:
    """Retrieves nodes based on optional status and enabled filters."""
    query = db.query(Node)

    if status:
        if isinstance(status, list):
            query = query.filter(Node.status.in_(status))
        else:
            query = query.filter(Node.status == status)
    if enabled:
        query = query.filter(Node.status.notin_([NodeStatus.disabled, NodeStatus.limited]))

    return query.all()
