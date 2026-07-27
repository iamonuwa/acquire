# Running cail-acquire

Runs the poller once a day against the live NL source for about a week, so we can
see how often its hash changes when the document itself hasn't.

**It must run on poppler 24.02** (what Ubuntu 24.04 ships). A different poppler
produces a different hash, so the numbers wouldn't mean anything — your Mac's
newer version won't do. Use an Ubuntu 24.04 host.

## 1. Build

```bash
make build-linux        # bin/cail-acquire-linux-amd64   (ARM host: make build-linux GOARCH=arm64)
```

## 2. Set up the host (Ubuntu 24.04, as root)

```bash
apt-get install -y poppler-utils git ca-certificates curl
pdftotext -v            # should say 24.02
adduser --system --group --no-create-home --shell /usr/sbin/nologin cail
install -d -o cail -g cail -m 0750 /var/lib/cail-acquire/cail-rules
install -d -o root -g cail -m 0750 /etc/cail-acquire
```

## 3. Copy the files (from your machine)

```bash
H=root@YOUR_HOST
scp bin/cail-acquire-linux-amd64    $H:/usr/local/bin/cail-acquire
scp deploy/cail-alert.sh            $H:/usr/local/bin/cail-alert.sh
scp deploy/*.service deploy/*.timer $H:/etc/systemd/system/
scp deploy/sources.seed.yaml        $H:/var/lib/cail-acquire/cail-rules/sources.yaml
scp deploy/env.example              $H:/etc/cail-acquire/env
```

Then on the host:

```bash
chmod 600 /etc/cail-acquire/env && chown root:cail /etc/cail-acquire/env
chown -R cail:cail /var/lib/cail-acquire/cail-rules
$EDITOR /etc/cail-acquire/env                     # add your R2 keys
$EDITOR /etc/systemd/system/cail-acquire.service  # set your hc-ping URL, or delete that line
```

## 4. Start it

```bash
systemctl daemon-reload
systemctl start cail-acquire.service    # run once by hand; first run prints change=true
journalctl -u cail-acquire.service -n 30
systemctl enable --now cail-acquire.timer
```

## What to expect

Most days it prints `change=false` and does nothing — that's the whole point. A
`change=true` on a day NL didn't actually republish is noise worth writing down;
we filter that out later. R2 only grows when the content really changes. If a run
fails it triggers the alert service — set `ALERT_WEBHOOK` to get pinged, otherwise
read the journal.

Give it about a week, then look at how much it churned.
