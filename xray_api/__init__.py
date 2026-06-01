"""Deprecated Python Xray API bindings.

The master no longer talks to Xray directly. New runtime and stats paths go
through Rebecca Node over Go/gRPC. This package is kept for model/type
compatibility while the remaining Python data models are migrated.
"""

from . import exceptions
from . import exceptions as exc
from . import types
from .proxyman import Proxyman
from .stats import Stats


class XRay(Proxyman, Stats):
    pass


__all__ = ["XRay", "exceptions", "exc", "types"]
