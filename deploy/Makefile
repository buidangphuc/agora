# Local GitOps deploy plane (EKS/ECR/ALB/ArgoCD stand-ins). See README.md.
.PHONY: cluster-up cluster-down images argocd-ui gitops-push smoke status release-tag smoke-staging status-staging

cluster-up:            ## kind + registry + ingress + argocd + gitea + app-of-apps
	bash kind/bootstrap.sh
	bash gitea/bootstrap-gitea.sh

images:                ## retag local service images -> registry (ECR) and push
	bash images/push-images.sh

release-tag:           ## retag images with the commit sha + bump envs/$$RELEASE_ENV/values.yaml  (TAG=<sha> RELEASE_ENV=staging DRY_RUN=1)
	bash images/release-tag.sh

gitops-push:           ## publish platform-gitops to LOCAL Gitea only (scratch clone; no commit/push in the agora repo)
	bash gitea/publish-gitops.sh

argocd-ui:             ## port-forward ArgoCD UI to https://localhost:8083 (admin / printed pw)
	@kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d; echo
	kubectl -n argocd port-forward svc/argocd-server 8083:443

status:                ## ArgoCD apps + marketplace pods
	kubectl -n argocd get applications -o wide
	kubectl -n marketplace get pods 2>/dev/null || true

smoke:                 ## run platform-e2e smoke against the cluster ingress
	cd ../platform-e2e && . .venv/bin/activate && \
	  BASE_URL=http://marketplace.localtest.me GATEWAY_URL=http://marketplace.localtest.me/api \
	  HEADLESS=true python -m pytest -m smoke -q

status-staging:        ## staging overlay on kind (ns marketplace-staging)
	kubectl -n argocd get applications -o wide | grep -E 'NAME|-staging' || true
	kubectl -n marketplace-staging get rollout,deploy,pods 2>/dev/null || true

smoke-staging:         ## wait for the staging overlay to be Healthy, then run platform-e2e smoke against it
	kubectl -n marketplace-staging rollout status deploy/team-frontend --timeout=180s
	kubectl -n marketplace-staging wait --for=jsonpath='{.status.phase}'=Healthy rollout/team-gateway --timeout=180s
	cd ../platform-e2e && . .venv/bin/activate && \
	  BASE_URL=http://marketplace-staging.localtest.me GATEWAY_URL=http://marketplace-staging.localtest.me/api \
	  HEADLESS=true python -m pytest -m smoke -q

cluster-down:          ## delete cluster + registry
	kind delete cluster --name marketplace || true
	docker rm -f kind-registry || true
