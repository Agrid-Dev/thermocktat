from types import TracebackType
from typing import Any, Self

from httpx import Client

from ._base import DEFAULT_TIMEOUT, NotSyncedError, ThermocktatClientBase
from .types import FanSpeed, Mode, Snapshot

__all__ = ["NotSyncedError", "ThermocktatSync"]


class ThermocktatSync(ThermocktatClientBase):
    def __init__(
        self,
        base_url: str,
        *,
        timeout: float = DEFAULT_TIMEOUT,
        client: Client | None = None,
    ) -> None:
        super().__init__(base_url)
        if client is not None:
            self._client = client
            self._owns_client = False
        else:
            self._client = Client(base_url=self.base_url, timeout=timeout)
            self._owns_client = True

    @classmethod
    def connect(
        cls,
        base_url: str,
        *,
        timeout: float = DEFAULT_TIMEOUT,
        client: Client | None = None,
    ) -> Self:
        instance = cls(base_url, timeout=timeout, client=client)
        instance.sync()
        return instance

    def close(self) -> None:
        if self._owns_client:
            self._client.close()

    def __enter__(self) -> Self:
        return self

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        tb: TracebackType | None,
    ) -> None:
        self.close()

    def sync(self) -> None:
        response = self._client.get(self._path())
        response.raise_for_status()
        self._update(Snapshot.from_dict(response.json()))

    def _set_value(self, attribute: str, value: Any) -> None:
        response = self._client.post(self._path(attribute), json={"value": value})
        response.raise_for_status()
        self._update(Snapshot.from_dict(response.json()))

    def set_enabled(self, value: bool) -> None:
        self._set_value("enabled", value)

    def set_temperature_setpoint(self, value: float) -> None:
        self._set_value("temperature_setpoint", value)

    def set_temperature_setpoint_min(self, value: float) -> None:
        self._set_value("temperature_setpoint_min", value)

    def set_temperature_setpoint_max(self, value: float) -> None:
        self._set_value("temperature_setpoint_max", value)

    def set_mode(self, value: Mode | str) -> None:
        self._set_value("mode", Mode(value))

    def set_fan_speed(self, value: FanSpeed | str) -> None:
        self._set_value("fan_speed", FanSpeed(value))

    def set_fault_code(self, value: int) -> None:
        self._set_value("fault_code", value)
