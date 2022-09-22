IMAGE:= databus23/groupcache-demo


build:
	docker build -t $(IMAGE) .


push: build
	docker push $(IMAGE)
