from datetime import datetime

from .types import Snapshot

API_VERSION = "v1"
DEFAULT_TIMEOUT = 10.0


class NotSyncedError(RuntimeError):
    """Raised when snapshot state is read before the first sync."""


class ThermocktatClientBase:
    """Shared state + URL logic between sync and async clients.

    Subclasses own HTTP client construction and I/O methods; this base owns
    the state contract (snapshot, last_synced, snapshot_age_seconds) and path building.
    """

    def __init__(self, base_url: str) -> None:
        self.base_url = base_url.rstrip("/")
        self._snapshot: Snapshot | None = None
        self._last_synced: datetime | None = None

    def _path(self, *parts: str) -> str:
        return "/" + "/".join((API_VERSION, *parts))

    def _update(self, snapshot: Snapshot) -> None:
        self._snapshot = snapshot
        self._last_synced = datetime.now()

    @property
    def snapshot(self) -> Snapshot:
        if self._snapshot is None:
            raise NotSyncedError(
                "client has not synced yet; use .connect(...) or call .sync()"
            )
        return self._snapshot

    @property
    def last_synced(self) -> datetime:
        if self._last_synced is None:
            raise NotSyncedError(
                "client has not synced yet; use .connect(...) or call .sync()"
            )
        return self._last_synced

    @property
    def snapshot_age_seconds(self) -> float:
        return (datetime.now() - self.last_synced).total_seconds()
