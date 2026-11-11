PREFIX       ?= /opt/gocd
# the service runs as whoever invoked sudo, so it uses their git credentials
SERVICE_USER ?= $(or $(SUDO_USER),$(USER))
UNIT         := /etc/systemd/system/gocd.service

.PHONY: build install service uninstall secret

build:
	go build -o gocd ./cmd/gocd/

install: build
	mkdir -p $(PREFIX)
	install -m 755 gocd $(PREFIX)/gocd
	ln -sf $(PREFIX)/gocd /usr/local/bin/gocd
	@if [ ! -f $(PREFIX)/config.yaml ] && [ -f config.yaml ]; then install -m 644 config.yaml $(PREFIX)/config.yaml; fi
	chown -R $(SERVICE_USER) $(PREFIX)

service: install
	printf '%s\n' \
		'[Unit]' \
		'Description=GoCD auto-pull' \
		'After=network-online.target' \
		'Wants=network-online.target' \
		'' \
		'[Service]' \
		'User=$(SERVICE_USER)' \
		'ExecStart=$(PREFIX)/gocd' \
		'Restart=always' \
		'RestartSec=10' \
		'' \
		'[Install]' \
		'WantedBy=multi-user.target' > $(UNIT)
	systemctl daemon-reload
	systemctl enable --now gocd
	systemctl restart gocd

secret:
	@if [ -f webhook.secret ]; then echo "webhook.secret already exists, delete it first to make a new one"; else umask 077 && openssl rand -hex 32 > webhook.secret; fi
	@cat webhook.secret

uninstall:
	-systemctl disable --now gocd
	rm -f $(UNIT)
	systemctl daemon-reload
	rm -rf $(PREFIX)
	rm -f /usr/local/bin/gocd
