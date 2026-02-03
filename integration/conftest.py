import os
import subprocess

import pytest


@pytest.fixture(scope="module")
def tmk_application():
    """Fixture to start the TMK application before tests and stop it after tests."""
    env = os.environ.copy()
    env["TMK_CONTROLLER"] = "http"
    env["TMK_CONTROLLERS_HTTP_ADDR"] = ":8080"
    env["TMK_REGULATOR_MODE_CHANGE_HYSTERESIS"] = "2.0"
    env["TMK_REGULATOR_TARGET_HYSTERESIS"] = "1.0"
    process = subprocess.Popen(
        ["./.bin/thermocktat"], env=env, stdout=None, stderr=None
    )

    yield  # Tests will run after this point

    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
