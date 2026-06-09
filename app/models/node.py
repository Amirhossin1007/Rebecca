from enum import Enum
from typing import Optional
from pydantic import ConfigDict, BaseModel


class NodeStatus(str, Enum):
    connected = "connected"
    connecting = "connecting"
    error = "error"
    disabled = "disabled"
    limited = "limited"


class GeoMode(str, Enum):
    default = "default"
    custom = "custom"


class XrayConfigMode(str, Enum):
    default = "default"
    custom = "custom"


class Node(BaseModel):
    name: str
    address: str
    port: int = 62050
    api_port: int = 62051
    usage_coefficient: float = 1.0
    data_limit: Optional[int] = None
    use_nobetci: bool = False
    nobetci_port: Optional[int] = None
    proxy_enabled: bool = False
    proxy_type: Optional[str] = None
    proxy_host: Optional[str] = None
    proxy_port: Optional[int] = None
    proxy_username: Optional[str] = None
    proxy_password: Optional[str] = None


class NodeResponse(Node):
    id: int
    xray_version: Optional[str] = None
    node_service_version: Optional[str] = None
    node_install_mode: Optional[str] = None
    node_binary_tag: Optional[str] = None
    node_update_channel: Optional[str] = None
    cpu_cores: Optional[int] = None
    cpu_frequency_hz: Optional[float] = None
    cpu_usage_percent: Optional[float] = None
    memory_used: Optional[int] = None
    memory_total: Optional[int] = None
    memory_usage_percent: Optional[float] = None
    upload_speed: Optional[int] = None
    download_speed: Optional[int] = None
    status: NodeStatus
    message: Optional[str] = None
    geo_mode: GeoMode
    xray_config_mode: XrayConfigMode = XrayConfigMode.default
    uplink: int = 0
    downlink: int = 0
    has_custom_certificate: bool = False
    uses_default_certificate: bool = False
    certificate_public_key: Optional[str] = None
    node_certificate: Optional[str] = None
    model_config = ConfigDict(from_attributes=True)


class NodeUsageResponse(BaseModel):
    node_id: Optional[int] = None
    node_name: str
    uplink: int
    downlink: int


class NodesUsageResponse(BaseModel):
    usages: list[NodeUsageResponse]
