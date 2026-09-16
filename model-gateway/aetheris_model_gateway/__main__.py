from .config import load
from .http import create_server, _embedding_provider_from_config, _provider_from_config


def build_server(config):
    return create_server(
        config.host,
        config.port,
        token=config.token,
        max_request_bytes=config.max_request_bytes,
        provider=_provider_from_config(config),
        embedding_provider=_embedding_provider_from_config(config),
    )


def main():
    config = load()
    server = build_server(config)
    try:
        server.serve_forever()
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
