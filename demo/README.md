Demo
====

Create a cluster

	curl -X POST localhost:8080/couchdb\?name=couchdb-cluster0

Create a document in the cluster

	./create_in_cluster.sh couchdb-cluster0

Retrieve from cluster

	./get couchdb-cluster0 document_id

Retrieve from pods

	./get_from_node.sh couchdb-cluster0 document_id pod_name

Delete the cluster

	curl -X DELETE localhost:8080/couchdb/couchdb-cluster0
