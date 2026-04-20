from types import TracebackType
from typing import Any, Self

from httpx import AsyncClient

from ._base import DEFAULT_TIMEOUT, NotSyncedError, ThermocktatClientBase
from .types import FanSpeed, Mode, Snapshot

__all__ = ["NotSyncedError", "ThermocktatAsync"]


class ThermocktatAsync(ThermocktatClientBase):
    def __init__(
        self,
        base_url: str,
        *,
        timeout: float = DEFAULT_TIMEOUT,
        client: AsyncClient | None = None,
    ) -> None:
        super().__init__(base_url)
        if client is not None:
            self._client = client
            self._owns_client = False
        else:
            self._client = AsyncClient(base_url=self.base_url, timeout=timeout)
            self._owns_client = True

    @classmethod
    async def connect(
        cls,
        base_url: str,
        *,
        timeout: float = DEFAULT_TIMEOUT,
        client: AsyncClient | None = None,
    ) -> Self:
        instance = cls(base_url, timeout=timeout, client=client)
        await instance.sync()
        return instance

    async def aclose(self) -> None:
        if self._owns_client:
            await self._client.aclose()

    async def __aenter__(self) -> Self:
        return self

    async def __aexit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        tb: TracebackType | None,
    ) -> None:
        await self.aclose()

    async def sync(self) -> None:
        response = await self._client.get(self._path())
        response.raise_for_status()
        self._update(Snapshot.from_dict(response.json()))

    async def _set_value(self, attribute: str, value: Any) -> None:
        response = await self._client.post(self._path(attribute), json={"value": value})
        response.raise_for_status()
        self._update(Snapshot.from_dict(response.json()))

    async def set_enabled(self, value: bool) -> None:
        await self._set_value("enabled", value)

    async def set_temperature_setpoint(self, value: float) -> None:
        await self._set_value("temperature_setpoint", value)

    async def set_temperature_setpoint_min(self, value: float) -> None:
        await self._set_value("temperature_setpoint_min", value)

    async def set_temperature_setpoint_max(self, value: float) -> None:
        await self._set_value("temperature_setpoint_max", value)

    async def set_mode(self, value: Mode | str) -> None:
        await self._set_value("mode", Mode(value))

    async def set_fan_speed(self, value: FanSpeed | str) -> None:
        await self._set_value("fan_speed", FanSpeed(value))

    async def set_fault_code(self, value: int) -> None:
        await self._set_value("fault_code", value)
