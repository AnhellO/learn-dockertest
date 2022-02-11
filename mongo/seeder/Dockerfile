FROM mongo

COPY addresses.json /addresses.json

CMD mongoimport --uri "mongodb://mongoadmin:secret@mongodb:27017/test_db?authSource=admin&readPreference=primary&directConnection=true&ssl=false" --db test_db --collection addresses --drop --type json --file /addresses.json