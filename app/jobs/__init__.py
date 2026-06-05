import importlib


JOB_MODULES = (
    "add_db_users",
    "remove_expired_users",
    "send_notifications",
)


for module_name in JOB_MODULES:
    importlib.import_module(f"{__name__}.{module_name}")
