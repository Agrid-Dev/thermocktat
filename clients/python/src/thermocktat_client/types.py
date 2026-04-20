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
    fault_code: int = 0

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Self:
        known = {f.name for f in fields(cls)}
        return cls(**{k: v for k, v in data.items() if k in known})

    def __str__(self) -> str:
        state = "on" if self.enabled else "off"
        return (
            f"Thermocktat[{self.device_id}] ({state})\n"
            f"  mode:     {self.mode.value}\n"
            f"  fan:      {self.fan_speed.value}\n"
            f"  setpoint: {self.temperature_setpoint}°C "
            f"(min {self.temperature_setpoint_min}, max {self.temperature_setpoint_max})\n"
            f"  ambient:  {self.ambient_temperature:.2f}°C\n"
            f"  fault:    {self.fault_code}"
        )
