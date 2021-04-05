
With the following sequence of events for `default/web` RS:

```
podname=web-g9s2p, creationTimestamp=2021-04-02 12:06:48 +0200 CEST, deletionTimestamp=2021-04-02 12:36:58 +0200 CEST
podname=web-phgsz, creationTimestamp=2021-04-02 12:18:08 +0200 CEST, deletionTimestamp=2021-04-02 12:36:49 +0200 CEST
podname=web-58tfs, creationTimestamp=2021-04-02 12:18:15 +0200 CEST, deletionTimestamp=<nil>
podname=web-gcq67, creationTimestamp=2021-04-02 12:18:24 +0200 CEST, deletionTimestamp=2021-04-02 12:36:58 +0200 CEST
podname=web-8vzsr, creationTimestamp=2021-04-02 12:18:25 +0200 CEST, deletionTimestamp=2021-04-02 12:36:45 +0200 CEST
podname=web-2bpd8, creationTimestamp=2021-04-02 12:36:45 +0200 CEST, deletionTimestamp=<nil>
podname=web-nhtrt, creationTimestamp=2021-04-02 12:36:49 +0200 CEST, deletionTimestamp=<nil>
podname=web-27r5t, creationTimestamp=2021-04-02 12:36:58 +0200 CEST, deletionTimestamp=2021-04-02 12:37:01 +0200 CEST
podname=web-bkxdk, creationTimestamp=2021-04-02 12:36:58 +0200 CEST, deletionTimestamp=<nil>
podname=web-28r5t, creationTimestamp=2021-04-02 12:37:02 +0200 CEST, deletionTimestamp=<nil>
```

Correlated chains (searching for the closest created pod for each deleted pod):

```
default/ReplicaSet/web/web-8vzsr -> default/ReplicaSet/web/web-2bpd8
default/ReplicaSet/web/web-phgsz -> default/ReplicaSet/web/web-nhtrt
default/ReplicaSet/web/web-gcq67 -> default/ReplicaSet/web/web-bkxdk
default/ReplicaSet/web/web-g9s2p -> default/ReplicaSet/web/web-27r5t -> default/ReplicaSet/web/web-28r5t
```

E.g. `web-8vzsr` got deleted at `12:36:45` and at `12:36:45` replaced with `web-2bpd8`.

Conclusion: pod `web-g9s2p` got re-created twice

**Application**: monitor how many times a given pod got replaced

**Use case**: detect pods getting recreated too many times during cluster upgrades
