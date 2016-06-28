package main

import (
	"fmt"
	"github.com/flower-pot/k8sdb/couchdb"
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
)

func CreateCouchdbCluster(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	queryValues := r.URL.Query()
	clusterName := queryValues.Get("name")

	if clusterName == "" {
		fmt.Fprint(w, `{"error":"Name must be passed via query params"}`)
		return
	}

	err := couchdb.CreateCluster(clusterName)
	if err != nil {
		fmt.Println("ERROR!")
		fmt.Println(err)
	}

	fmt.Fprint(w, `{"status":"Creating"}`)
}

func DeleteCouchdbCluster(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	err := couchdb.DeleteCluster(ps.ByName("cluster_id"))
	if err != nil {
		fmt.Println("ERROR!")
		fmt.Println(err)
	}

	fmt.Fprint(w, `{"status":"Deleting"}`)
}

func main() {
	router := httprouter.New()
	router.POST("/couchdb", CreateCouchdbCluster)
	router.DELETE("/couchdb/:cluster_id", DeleteCouchdbCluster)

	log.Fatal(http.ListenAndServe(":8080", router))
}
