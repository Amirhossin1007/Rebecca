from pydantic import BaseModel


class RuntimeStats(BaseModel):
    version: str | None
    started: bool
    logs_websocket: str


class ServerIPs(BaseModel):
    ipv4: str
    ipv6: str
