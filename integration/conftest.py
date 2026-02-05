import os
import subprocess
import time

import pytest


@pytest.fixture(scope="module")
def tmk_application():
    """Fixture to start the TMK application before tests and stop it after tests."""
    env = os.environ.copy()
    # Enable HTTP controller
    env["TMK_CONTROLLERS_HTTP_ENABLED"] = "true"
    env["TMK_CONTROLLERS_HTTP_ADDR"] = ":8080"

    # Enable Modbus controller
    env["TMK_CONTROLLERS_MODBUS_ENABLED"] = "true"
    env["TMK_CONTROLLERS_MODBUS_ADDR"] = "0.0.0.0:1502"
    env["TMK_CONTROLLERS_MODBUS_UNIT_ID"] = "4"

    # Regulator settings
    env["TMK_REGULATOR_MODE_CHANGE_HYSTERESIS"] = "2.0"
    env["TMK_REGULATOR_TARGET_HYSTERESIS"] = "1.0"
    process = subprocess.Popen(
        ["./.bin/thermocktat"], env=env, stdout=None, stderr=None
    )
    time.sleep(2)
    yield  # Tests will run after this point

    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
