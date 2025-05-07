document.addEventListener('DOMContentLoaded', () => {
    const aircraftFeatures = new Map();

    const aircraftIconBlueURI = 'data:image/svg+xml;charset=UTF-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20viewBox%3D%220%200%2024%2024%22%20width%3D%2228px%22%20height%3D%2228px%22%20fill%3D%22%23007bff%22%3E%3Cpath%20d%3D%22M21%2016v-2l-8-5V3.5c0-.83-.67-1.5-1.5-1.5S10%202.67%2010%203.5V9l-8%205v2l8-2.5V19l-2%201.5V22l3.5-1%203.5%201v-1.5L13%2019v-5.5l8%202.5z%22%2F%3E%3C%2Fsvg%3E';
    const aircraftIconRedURI = 'data:image/svg+xml;charset=UTF-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20viewBox%3D%220%200%2024%2024%22%20width%3D%2232px%22%20height%3D%2232px%22%20fill%3D%22%23dc3545%22%3E%3Cpath%20d%3D%22M21%2016v-2l-8-5V3.5c0-.83-.67-1.5-1.5-1.5S10%202.67%2010%203.5V9l-8%205v2l8-2.5V19l-2%201.5V22l3.5-1%203.5%201v-1.5L13%2019v-5.5l8%202.5z%22%2F%3E%3C%2Fsvg%3E';

    function getAircraftStyle(feature) {
        const isAlerted = feature.get('alerted');
        const track = feature.get('track') || 0;
        const rotation = track * Math.PI / 180;

        return new ol.style.Style({
            image: new ol.style.Icon({
                anchor: [0.5, 0.5],
                anchorXUnits: 'fraction',
                anchorYUnits: 'fraction',
                src: isAlerted ? aircraftIconRedURI : aircraftIconBlueURI,
                scale: 1.2,
                rotation: rotation,
                rotateWithView: true
            })
        });
    }

    const aircraftVectorSource = new ol.source.Vector();
    const aircraftVectorLayer = new ol.layer.Vector({
        source: aircraftVectorSource,
        style: getAircraftStyle
    });

    const map = new ol.Map({
        target: 'map',
        layers: [
            new ol.layer.Tile({
                source: new ol.source.OSM()
            }),
            aircraftVectorLayer
        ],
        view: new ol.View({
            center: ol.proj.fromLonLat([0, 0]),
            zoom: 2
        })
    });

    const alertList = document.getElementById('alert-list');
    let activeAlertICAOs = new Set();

    function addAlertToList(alert) {
        const listItem = document.createElement('li');
        listItem.textContent = `ALERT (${alert.criteria.callsign || alert.criteria.icao}): ${alert.message} (Aircraft: ${alert.aircraft.callsign}/${alert.aircraft.icao}) at ${new Date(alert.timestamp).toLocaleString()}`;
        alertList.insertBefore(listItem, alertList.firstChild);
        
        activeAlertICAOs.add(alert.aircraft.icao);
        const feature = aircraftFeatures.get(alert.aircraft.icao);
        if (feature) {
            feature.set('alerted', true);
            feature.changed();
        }
    }
    
    const AIRCRAFT_TIMEOUT_MS = 60000;
    setInterval(() => {
        const now = Date.now();
        aircraftFeatures.forEach((feature, icao) => {
            const lastUpdate = feature.get('lastUpdate');
            if (now - lastUpdate > AIRCRAFT_TIMEOUT_MS && !activeAlertICAOs.has(icao)) {
                aircraftVectorSource.removeFeature(feature);
                aircraftFeatures.delete(icao);
                console.log(`Removed stale aircraft ${icao} from map.`);
            }
        });
    }, 30000);

    const ALERT_DISPLAY_DURATION_MS = 120000;
    setInterval(() => {
        activeAlertICAOs.forEach(icao => {
            const feature = aircraftFeatures.get(icao);
            if (feature) {
            }
        });
    }, ALERT_DISPLAY_DURATION_MS / 2);

    function clearAlertedStatus(icao) {
        activeAlertICAOs.delete(icao);
        const feature = aircraftFeatures.get(icao);
        if (feature) {
            feature.set('alerted', false);
            feature.changed();
        }
    }

    console.log("Attempting to connect to SSE at /api/events");
    const eventSource = new EventSource('/api/events');

    eventSource.onopen = function() {
        console.log("SSE connection opened successfully.");
        alertList.innerHTML = ''; 
        const listItem = document.createElement('li');
        listItem.id = "sse-status-message";
        listItem.textContent = 'Connected to stream... Waiting for data.';
        alertList.appendChild(listItem);
    };

    eventSource.onerror = function(err) {
        console.error("EventSource failed:", err);
        alertList.innerHTML = ''; 
        const listItem = document.createElement('li');
        listItem.textContent = 'Error connecting to stream. Please refresh.';
        alertList.appendChild(listItem);
    };

    eventSource.addEventListener('alert', function(event) {
        console.log("Received specific 'alert' event:", event.data);
        const statusMessage = document.getElementById("sse-status-message");
        if (statusMessage) statusMessage.remove();
        try {
            const alertData = JSON.parse(event.data);
            addAlertToList(alertData);
            setTimeout(() => clearAlertedStatus(alertData.aircraft.icao), ALERT_DISPLAY_DURATION_MS);
        } catch (e) {
            console.error("Error parsing alert data from 'alert' event:", e, "Raw data:", event.data);
        }
    });

    eventSource.addEventListener('aircraftUpdate', function(event) {
        const statusMessage = document.getElementById("sse-status-message");
        if (statusMessage) statusMessage.remove();
        try {
            const acData = JSON.parse(event.data);
            if (!acData.icao || typeof acData.lat !== 'number' || typeof acData.lon !== 'number') {
                console.warn("Received incomplete aircraft data:", acData);
                return;
            }

            let feature = aircraftFeatures.get(acData.icao);
            const coordinates = ol.proj.fromLonLat([acData.lon, acData.lat]);

            if (feature) {
                feature.getGeometry().setCoordinates(coordinates);
                feature.set('track', acData.track);
            } else {
                feature = new ol.Feature({
                    geometry: new ol.geom.Point(coordinates),
                    name: acData.callsign || acData.icao,
                    icao: acData.icao,
                    track: acData.track,
                    alerted: activeAlertICAOs.has(acData.icao)
                });
                aircraftVectorSource.addFeature(feature);
                aircraftFeatures.set(acData.icao, feature);
            }
            feature.set('lastUpdate', Date.now());
            if (!activeAlertICAOs.has(acData.icao)) {
                 feature.set('alerted', false);
            }
            feature.changed();

        } catch (e) {
            console.error("Error parsing aircraft data from 'aircraftUpdate' event:", e, "Raw data:", event.data);
        }
    });

    eventSource.onmessage = function(event) {
        if (event.type !== 'alert' && event.type !== 'aircraftUpdate') {
            console.log("Received generic SSE message (untyped or keep-alive?):", event);
        }
    };
});