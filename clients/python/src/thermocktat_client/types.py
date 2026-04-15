from dataclasses import dataclass, fields
from enum import StrEnum
from typing import Any, Self


class Mode(StrEnum):
    HEAT = "heat"
    COOL = "cool"
    AUTO = "auto"
    FAN = "fan"


class FanSpeed(StrEnum):
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    AUTO = "auto"


@dataclass
class Snapshot:
    device_id: str
    enabled: bool
    ambient_temperature: float
    temperature_setpoint: float
    temperature_setpoint_min: float
    temperature_setpoint_max: float
    mode: Mode
    fan_speed: FanSpeed

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Self:
        known = {f.name for f in fields(cls)}
        return cls(**{k: v for k, v in data.items() if k in known})
