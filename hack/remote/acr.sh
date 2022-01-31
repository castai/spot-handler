az acr create --resource-group $RESOURCE_GROUP --name $REGISTRY_NAME --sku Basic
az aks update -n $CLUSTER_NAME -g $RESOURCE_GROUP --attach-acr $REGISTRY_NAME
az acr login --name $REGISTRY_NAME