FROM mongo

COPY restaurants.json /restaurants.json

CMD mongoimport --uri "mongodb://mongoadmin:secret@mongodb:27017/test_db?authSource=admin&readPreference=primary&directConnection=true&ssl=false" --db test_db --collection restaurants --drop --type json --file /restaurants.json