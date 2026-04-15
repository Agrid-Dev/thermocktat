from datetime import datetime

import pytest
from pytest_httpx import HTTPXMock

from thermocktat_client.thermocktat_sync import NotSyncedError, ThermocktatSync

BASE_URL = "http://localhost:8080"


def test_cannot_connect_without_base_url():
    with pytest.raises(TypeError):
        _ = ThermocktatSync.connect()  # pyright: ignore[reportCallIssue]


def test_plain_construction_does_no_network_and_raises_on_read():
    tmk = ThermocktatSync(base_url=BASE_URL)
    with pytest.raises(NotSyncedError):
        _ = tmk.snapshot
    with pytest.raises(NotSyncedError):
        _ = tmk.last_synced
    with pytest.raises(NotSyncedError):
        _ = tmk.snapshot_age_seconds


@pytest.fixture
def tmk(snapshot_response, httpx_mock: HTTPXMock):
    httpx_mock.add_response(
        status_code=200, url=f"{BASE_URL}/v1", json=snapshot_response
    )
    return ThermocktatSync.connect(BASE_URL)


def test_connect_initializes_from_snapshot(tmk):
    assert tmk.snapshot.enabled
    assert tmk.snapshot.temperature_setpoint == 22
    assert tmk.snapshot.mode == "auto"


def test_connect_tolerates_unknown_fields(snapshot_response, httpx_mock: HTTPXMock):
    snapshot_response["newly_added_server_field"] = "ignored"
    httpx_mock.add_response(
        status_code=200, url=f"{BASE_URL}/v1", json=snapshot_response
    )
    tmk = ThermocktatSync.connect(BASE_URL)
    assert tmk.snapshot.enabled is True


def test_syncs_on_command(tmk, snapshot_response, httpx_mock: HTTPXMock):
    snapshot_response["temperature_setpoint"] = 23
    httpx_mock.add_response(
        status_code=200, url=f"{BASE_URL}/v1", json=snapshot_response
    )
    tmk.sync()
    assert tmk.snapshot.temperature_setpoint == 23


def test_has_last_synced(tmk):
    assert (datetime.now() - tmk.last_synced).total_seconds() < 0.1


def test_has_snapshot_age_seconds(tmk):
    assert tmk.snapshot_age_seconds < 0.1


def test_context_manager_closes_owned_client(snapshot_response, httpx_mock: HTTPXMock):
    httpx_mock.add_response(
        status_code=200, url=f"{BASE_URL}/v1", json=snapshot_response
    )
    with ThermocktatSync.connect(BASE_URL) as tmk:
        assert tmk.snapshot.enabled is True
    assert tmk._client.is_closed


def test_injected_client_is_not_closed(snapshot_response, httpx_mock: HTTPXMock):
    from httpx import Client

    external = Client(base_url=BASE_URL)
    httpx_mock.add_response(
        status_code=200, url=f"{BASE_URL}/v1", json=snapshot_response
    )
    with ThermocktatSync.connect(BASE_URL, client=external):
        pass
    assert not external.is_closed
    external.close()


@pytest.mark.parametrize(
    ("attr", "value"),
    [
        ("enabled", False),
        ("temperature_setpoint", 24),
        ("temperature_setpoint_min", 15),
        ("temperature_setpoint_max", 30),
        ("mode", "cool"),
        ("fan_speed", "high"),
    ],
)
def test_setters(
    tmk, snapshot_response, httpx_mock: HTTPXMock, attr: str, value
) -> None:
    snapshot_response[attr] = value
    httpx_mock.add_response(
        status_code=200, url=f"{BASE_URL}/v1/{attr}", json=snapshot_response
    )
    getattr(tmk, f"set_{attr}")(value)
    assert getattr(tmk.snapshot, attr) == value
