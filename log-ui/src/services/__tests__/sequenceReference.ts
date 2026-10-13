// Generated from troubleshooting-sequence-agent/fixtures/sequence-cases.json (sequence-workspace-fixture-v1),
// converted to the trace report API shapes. Synthetic presentation scenarios.
/* eslint-disable */
export const sequenceCases: Record<string, any> = {
 "error": {
  "id": "error",
  "label": "Error spans + error logs",
  "traceId": "7a4c9d02000000000000000000000001",
  "selectedSpanId": "0000000000000002",
  "release": "1.1.0",
  "timestampQuality": "comparable",
  "sources": {
   "spans": "available",
   "logs": "available"
  },
  "spans": [
   {
    "timeStamp": "1791792000000000000",
    "name": "publish_recording",
    "spanKind": "server",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000001",
    "parentSpanId": "0000000000000000",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 240000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000025000000",
    "name": "upload_recording",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000002",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 200000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000040000000",
    "name": "PutObject",
    "spanKind": "client",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000003",
    "parentSpanId": "0000000000000002",
    "msg": "",
    "tags": [
     {
      "key": "server.address",
      "value": "object-storage"
     },
     {
      "key": "http.response.status_code",
      "value": "403"
     }
    ],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 172000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000042000000",
    "name": "PUT /recordings",
    "spanKind": "server",
    "service": "object-storage",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000004",
    "parentSpanId": "0000000000000003",
    "msg": "",
    "tags": [
     {
      "key": "http.response.status_code",
      "value": "403"
     }
    ],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 168000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000030000000",
    "name": "GenerateThumbnail",
    "spanKind": "client",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000005",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 122000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000032000000",
    "name": "generate_thumbnail",
    "spanKind": "server",
    "service": "thumbnail-worker",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000006",
    "parentSpanId": "0000000000000005",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 118000,
    "events": [],
    "links": []
   }
  ],
  "logs": [
   {
    "timeUnixNano": "1791792000003000000",
    "recordId": "log-001",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000001",
    "severity": "INFO",
    "service": "",
    "message": "Recording request accepted",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000026000000",
    "recordId": "log-002",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Upload started",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000035000000",
    "recordId": "log-003",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Preparing metadata",
    "attributes": {
     "task.id": "metadata"
    }
   },
   {
    "timeUnixNano": "1791792000041000000",
    "recordId": "log-004",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000003",
    "severity": "INFO",
    "service": "",
    "message": "Sending object to storage",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000205000000",
    "recordId": "log-005",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000004",
    "severity": "ERROR",
    "service": "",
    "message": "AccessDenied: bucket policy rejected the request",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000211000000",
    "recordId": "log-006",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000003",
    "severity": "ERROR",
    "service": "",
    "message": "Storage returned HTTP 403",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000215000000",
    "recordId": "log-007",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000002",
    "severity": "ERROR",
    "service": "",
    "message": "Upload failed: AccessDenied",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000218000000",
    "recordId": "log-008",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000002",
    "severity": "ERROR",
    "service": "",
    "message": "Could not persist recording",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000080000000",
    "recordId": "log-009",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Metadata prepared",
    "attributes": {
     "task.id": "metadata"
    }
   },
   {
    "timeUnixNano": "1791792000149000000",
    "recordId": "log-010",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000006",
    "severity": "INFO",
    "service": "",
    "message": "Thumbnail generated",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000235000000",
    "recordId": "log-011",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "0000000000000001",
    "severity": "WARN",
    "service": "",
    "message": "Recording was not published",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000237000000",
    "recordId": "log-012",
    "traceId": "7a4c9d02000000000000000000000001",
    "spanId": "",
    "severity": "WARN",
    "service": "",
    "message": "Trace-level audit event: publication incomplete",
    "attributes": {}
   }
  ]
 },
 "logs-only": {
  "id": "logs-only",
  "label": "OK spans + error logs",
  "traceId": "7a4c9d02000000000000000000000002",
  "selectedSpanId": "0000000000000002",
  "release": "1.1.0",
  "timestampQuality": "comparable",
  "sources": {
   "spans": "available",
   "logs": "available"
  },
  "spans": [
   {
    "timeStamp": "1791792000000000000",
    "name": "publish_recording",
    "spanKind": "server",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000001",
    "parentSpanId": "0000000000000000",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 240000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000025000000",
    "name": "upload_recording",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000002",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 200000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000040000000",
    "name": "PutObject",
    "spanKind": "client",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000003",
    "parentSpanId": "0000000000000002",
    "msg": "",
    "tags": [
     {
      "key": "server.address",
      "value": "object-storage"
     },
     {
      "key": "http.response.status_code",
      "value": "403"
     }
    ],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 172000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000042000000",
    "name": "PUT /recordings",
    "spanKind": "server",
    "service": "object-storage",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000004",
    "parentSpanId": "0000000000000003",
    "msg": "",
    "tags": [
     {
      "key": "http.response.status_code",
      "value": "403"
     }
    ],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 168000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000030000000",
    "name": "GenerateThumbnail",
    "spanKind": "client",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000005",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 122000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000032000000",
    "name": "generate_thumbnail",
    "spanKind": "server",
    "service": "thumbnail-worker",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000006",
    "parentSpanId": "0000000000000005",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 118000,
    "events": [],
    "links": []
   }
  ],
  "logs": [
   {
    "timeUnixNano": "1791792000003000000",
    "recordId": "log-001",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000001",
    "severity": "INFO",
    "service": "",
    "message": "Recording request accepted",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000026000000",
    "recordId": "log-002",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Upload started",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000035000000",
    "recordId": "log-003",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Preparing metadata",
    "attributes": {
     "task.id": "metadata"
    }
   },
   {
    "timeUnixNano": "1791792000041000000",
    "recordId": "log-004",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000003",
    "severity": "INFO",
    "service": "",
    "message": "Sending object to storage",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000205000000",
    "recordId": "log-005",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000004",
    "severity": "ERROR",
    "service": "",
    "message": "AccessDenied: bucket policy rejected the request",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000211000000",
    "recordId": "log-006",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000003",
    "severity": "ERROR",
    "service": "",
    "message": "Storage returned HTTP 403",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000215000000",
    "recordId": "log-007",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000002",
    "severity": "ERROR",
    "service": "",
    "message": "Upload failed: AccessDenied",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000218000000",
    "recordId": "log-008",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000002",
    "severity": "ERROR",
    "service": "",
    "message": "Could not persist recording",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000080000000",
    "recordId": "log-009",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Metadata prepared",
    "attributes": {
     "task.id": "metadata"
    }
   },
   {
    "timeUnixNano": "1791792000149000000",
    "recordId": "log-010",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000006",
    "severity": "INFO",
    "service": "",
    "message": "Thumbnail generated",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000235000000",
    "recordId": "log-011",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "0000000000000001",
    "severity": "WARN",
    "service": "",
    "message": "Recording was not published",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000237000000",
    "recordId": "log-012",
    "traceId": "7a4c9d02000000000000000000000002",
    "spanId": "",
    "severity": "WARN",
    "service": "",
    "message": "Trace-level audit event: publication incomplete",
    "attributes": {}
   }
  ]
 },
 "partial": {
  "id": "partial",
  "label": "Missing span + orphan logs",
  "traceId": "7a4c9d02000000000000000000000003",
  "selectedSpanId": "0000000000000003",
  "release": "1.1.0",
  "timestampQuality": "comparable",
  "sources": {
   "spans": "partial",
   "logs": "available"
  },
  "spans": [
   {
    "timeStamp": "1791792000000000000",
    "name": "publish_recording",
    "spanKind": "server",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000001",
    "parentSpanId": "0000000000000000",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 240000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000040000000",
    "name": "PutObject",
    "spanKind": "client",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000003",
    "parentSpanId": "0000000000000002",
    "msg": "",
    "tags": [
     {
      "key": "server.address",
      "value": "object-storage"
     },
     {
      "key": "http.response.status_code",
      "value": "403"
     }
    ],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 172000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000042000000",
    "name": "PUT /recordings",
    "spanKind": "server",
    "service": "object-storage",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000004",
    "parentSpanId": "0000000000000003",
    "msg": "",
    "tags": [
     {
      "key": "http.response.status_code",
      "value": "403"
     }
    ],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 168000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000030000000",
    "name": "GenerateThumbnail",
    "spanKind": "client",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000005",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 122000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000032000000",
    "name": "generate_thumbnail",
    "spanKind": "server",
    "service": "thumbnail-worker",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000006",
    "parentSpanId": "0000000000000005",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 118000,
    "events": [],
    "links": []
   }
  ],
  "logs": [
   {
    "timeUnixNano": "1791792000003000000",
    "recordId": "log-001",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000001",
    "severity": "INFO",
    "service": "",
    "message": "Recording request accepted",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000026000000",
    "recordId": "log-002",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Upload started",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000035000000",
    "recordId": "log-003",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Preparing metadata",
    "attributes": {
     "task.id": "metadata"
    }
   },
   {
    "timeUnixNano": "1791792000041000000",
    "recordId": "log-004",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000003",
    "severity": "INFO",
    "service": "",
    "message": "Sending object to storage",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000205000000",
    "recordId": "log-005",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000004",
    "severity": "ERROR",
    "service": "",
    "message": "AccessDenied: bucket policy rejected the request",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000211000000",
    "recordId": "log-006",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000003",
    "severity": "ERROR",
    "service": "",
    "message": "Storage returned HTTP 403",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000215000000",
    "recordId": "log-007",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000002",
    "severity": "ERROR",
    "service": "",
    "message": "Upload failed: AccessDenied",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000218000000",
    "recordId": "log-008",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000002",
    "severity": "ERROR",
    "service": "",
    "message": "Could not persist recording",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000080000000",
    "recordId": "log-009",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Metadata prepared",
    "attributes": {
     "task.id": "metadata"
    }
   },
   {
    "timeUnixNano": "1791792000149000000",
    "recordId": "log-010",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000006",
    "severity": "INFO",
    "service": "",
    "message": "Thumbnail generated",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000235000000",
    "recordId": "log-011",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "0000000000000001",
    "severity": "WARN",
    "service": "",
    "message": "Recording was not published",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000237000000",
    "recordId": "log-012",
    "traceId": "7a4c9d02000000000000000000000003",
    "spanId": "",
    "severity": "WARN",
    "service": "",
    "message": "Trace-level audit event: publication incomplete",
    "attributes": {}
   }
  ]
 },
 "logs-unavailable": {
  "id": "logs-unavailable",
  "label": "Log source unavailable",
  "traceId": "7a4c9d02000000000000000000000004",
  "selectedSpanId": "0000000000000002",
  "release": "1.1.0",
  "timestampQuality": "comparable",
  "sources": {
   "spans": "available",
   "logs": "unavailable"
  },
  "spans": [
   {
    "timeStamp": "1791792000000000000",
    "name": "publish_recording",
    "spanKind": "server",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000004",
    "spanId": "0000000000000001",
    "parentSpanId": "0000000000000000",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 240000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000025000000",
    "name": "upload_recording",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000004",
    "spanId": "0000000000000002",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 200000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000040000000",
    "name": "PutObject",
    "spanKind": "client",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000004",
    "spanId": "0000000000000003",
    "parentSpanId": "0000000000000002",
    "msg": "",
    "tags": [
     {
      "key": "server.address",
      "value": "object-storage"
     },
     {
      "key": "http.response.status_code",
      "value": "403"
     }
    ],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 172000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000042000000",
    "name": "PUT /recordings",
    "spanKind": "server",
    "service": "object-storage",
    "traceId": "7a4c9d02000000000000000000000004",
    "spanId": "0000000000000004",
    "parentSpanId": "0000000000000003",
    "msg": "",
    "tags": [
     {
      "key": "http.response.status_code",
      "value": "403"
     }
    ],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 168000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000030000000",
    "name": "GenerateThumbnail",
    "spanKind": "client",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000004",
    "spanId": "0000000000000005",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 122000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000032000000",
    "name": "generate_thumbnail",
    "spanKind": "server",
    "service": "thumbnail-worker",
    "traceId": "7a4c9d02000000000000000000000004",
    "spanId": "0000000000000006",
    "parentSpanId": "0000000000000005",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 118000,
    "events": [],
    "links": []
   }
  ],
  "logs": []
 },
 "expired": {
  "id": "expired",
  "label": "Raw evidence expired",
  "traceId": "7a4c9d02000000000000000000000005",
  "selectedSpanId": null,
  "release": "1.1.0",
  "timestampQuality": "comparable",
  "sources": {
   "spans": "expired",
   "logs": "expired"
  },
  "spans": [],
  "logs": []
 },
 "large": {
  "id": "large",
  "label": "Large trace · 126 spans",
  "traceId": "7a4c9d02000000000000000000000006",
  "selectedSpanId": "0000000000000002",
  "release": "1.1.0",
  "timestampQuality": "comparable",
  "sources": {
   "spans": "available",
   "logs": "available"
  },
  "spans": [
   {
    "timeStamp": "1791792000000000000",
    "name": "publish_recording",
    "spanKind": "server",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000001",
    "parentSpanId": "0000000000000000",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 240000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000025000000",
    "name": "upload_recording",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000002",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 200000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000040000000",
    "name": "PutObject",
    "spanKind": "client",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000003",
    "parentSpanId": "0000000000000002",
    "msg": "",
    "tags": [
     {
      "key": "server.address",
      "value": "object-storage"
     },
     {
      "key": "http.response.status_code",
      "value": "403"
     }
    ],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 172000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000042000000",
    "name": "PUT /recordings",
    "spanKind": "server",
    "service": "object-storage",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000004",
    "parentSpanId": "0000000000000003",
    "msg": "",
    "tags": [
     {
      "key": "http.response.status_code",
      "value": "403"
     }
    ],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 168000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000030000000",
    "name": "GenerateThumbnail",
    "spanKind": "client",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000005",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 122000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000032000000",
    "name": "generate_thumbnail",
    "spanKind": "server",
    "service": "thumbnail-worker",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000006",
    "parentSpanId": "0000000000000005",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 118000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000000000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000064",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000001000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000065",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000002000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000066",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000003000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000067",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000004000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000068",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000005000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000069",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000006000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000006a",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000007000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000006b",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000008000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000006c",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000009000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000006d",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000010000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000006e",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000011000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000006f",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000012000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000070",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000013000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000071",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000014000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000072",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000015000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000073",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000016000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000074",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000017000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000075",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000018000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000076",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000019000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000077",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000020000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000078",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000021000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000079",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000022000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000007a",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000023000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000007b",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000024000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000007c",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000025000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000007d",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000026000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000007e",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000027000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000007f",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000028000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000080",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000029000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000081",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000030000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000082",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000031000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000083",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000032000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000084",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000033000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000085",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000034000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000086",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000035000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000087",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000036000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000088",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000037000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000089",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000038000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000008a",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000039000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000008b",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000040000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000008c",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000041000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000008d",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000042000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000008e",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000043000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000008f",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000044000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000090",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000045000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000091",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000046000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000092",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000047000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000093",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000048000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000094",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000049000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000095",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000050000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000096",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000051000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000097",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000052000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000098",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000053000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000099",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000054000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000009a",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000055000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000009b",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000056000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000009c",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000057000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000009d",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000058000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000009e",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000059000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "000000000000009f",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000060000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000a0",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000061000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000a1",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000062000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000a2",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000063000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000a3",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000064000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000a4",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000065000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000a5",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000066000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000a6",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000067000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000a7",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000068000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000a8",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000069000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000a9",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000070000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000aa",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000071000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000ab",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000072000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000ac",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000073000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000ad",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000074000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000ae",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000075000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000af",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000076000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000b0",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000077000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000b1",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000078000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000b2",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000079000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000b3",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000080000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000b4",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000081000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000b5",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000082000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000b6",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000083000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000b7",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000084000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000b8",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000085000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000b9",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000086000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000ba",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000087000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000bb",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000088000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000bc",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000089000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000bd",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000090000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000be",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000091000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000bf",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000092000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000c0",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000093000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000c1",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000094000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000c2",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000095000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000c3",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000096000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000c4",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000097000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000c5",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000098000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000c6",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000099000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000c7",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000100000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000c8",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000101000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000c9",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000102000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000ca",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000103000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000cb",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000104000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000cc",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000105000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000cd",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000106000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000ce",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000107000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000cf",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000108000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000d0",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000109000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000d1",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000110000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000d2",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000111000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000d3",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000112000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000d4",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000113000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000d5",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000114000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000d6",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000115000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000d7",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000116000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000d8",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000117000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000d9",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000118000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000da",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000119000000",
    "name": "validate_chunk",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "00000000000000db",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 2000,
    "events": [],
    "links": []
   }
  ],
  "logs": [
   {
    "timeUnixNano": "1791792000003000000",
    "recordId": "log-001",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000001",
    "severity": "INFO",
    "service": "",
    "message": "Recording request accepted",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000026000000",
    "recordId": "log-002",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Upload started",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000035000000",
    "recordId": "log-003",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Preparing metadata",
    "attributes": {
     "task.id": "metadata"
    }
   },
   {
    "timeUnixNano": "1791792000041000000",
    "recordId": "log-004",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000003",
    "severity": "INFO",
    "service": "",
    "message": "Sending object to storage",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000205000000",
    "recordId": "log-005",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000004",
    "severity": "ERROR",
    "service": "",
    "message": "AccessDenied: bucket policy rejected the request",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000211000000",
    "recordId": "log-006",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000003",
    "severity": "ERROR",
    "service": "",
    "message": "Storage returned HTTP 403",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000215000000",
    "recordId": "log-007",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000002",
    "severity": "ERROR",
    "service": "",
    "message": "Upload failed: AccessDenied",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000218000000",
    "recordId": "log-008",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000002",
    "severity": "ERROR",
    "service": "",
    "message": "Could not persist recording",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000080000000",
    "recordId": "log-009",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Metadata prepared",
    "attributes": {
     "task.id": "metadata"
    }
   },
   {
    "timeUnixNano": "1791792000149000000",
    "recordId": "log-010",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000006",
    "severity": "INFO",
    "service": "",
    "message": "Thumbnail generated",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000235000000",
    "recordId": "log-011",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "0000000000000001",
    "severity": "WARN",
    "service": "",
    "message": "Recording was not published",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000237000000",
    "recordId": "log-012",
    "traceId": "7a4c9d02000000000000000000000006",
    "spanId": "",
    "severity": "WARN",
    "service": "",
    "message": "Trace-level audit event: publication incomplete",
    "attributes": {}
   }
  ]
 },
 "async": {
  "id": "async",
  "label": "Async link without dependency semantics",
  "traceId": "7a4c9d02000000000000000000000007",
  "selectedSpanId": "0000000000000015",
  "release": "1.1.0",
  "timestampQuality": "comparable",
  "sources": {
   "spans": "available",
   "logs": "available"
  },
  "spans": [
   {
    "timeStamp": "1791792000000000000",
    "name": "submit_job",
    "spanKind": "server",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000007",
    "spanId": "0000000000000014",
    "parentSpanId": "0000000000000000",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 30000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000005000000",
    "name": "publish_job",
    "spanKind": "producer",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000007",
    "spanId": "0000000000000015",
    "parentSpanId": "0000000000000014",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 15000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000100000000",
    "name": "process_job",
    "spanKind": "consumer",
    "service": "worker",
    "traceId": "7a4c9d02000000000000000000000007",
    "spanId": "0000000000000016",
    "parentSpanId": "0000000000000014",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 130000,
    "events": [],
    "links": [
     {
      "traceId": "7a4c9d02000000000000000000000007",
      "spanId": "0000000000000015",
      "traceState": "",
      "attributes": {}
     }
    ]
   }
  ],
  "logs": []
 },
 "clock-uncertain": {
  "id": "clock-uncertain",
  "label": "Clock ordering uncertain",
  "traceId": "7a4c9d02000000000000000000000008",
  "selectedSpanId": "0000000000000002",
  "release": "1.1.0",
  "timestampQuality": "uncertain",
  "sources": {
   "spans": "available",
   "logs": "available"
  },
  "spans": [
   {
    "timeStamp": "1791792000000000000",
    "name": "publish_recording",
    "spanKind": "server",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000001",
    "parentSpanId": "0000000000000000",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 240000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000025000000",
    "name": "upload_recording",
    "spanKind": "internal",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000002",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 200000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000040000000",
    "name": "PutObject",
    "spanKind": "client",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000003",
    "parentSpanId": "0000000000000002",
    "msg": "",
    "tags": [
     {
      "key": "server.address",
      "value": "object-storage"
     },
     {
      "key": "http.response.status_code",
      "value": "403"
     }
    ],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 172000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000042000000",
    "name": "PUT /recordings",
    "spanKind": "server",
    "service": "object-storage",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000004",
    "parentSpanId": "0000000000000003",
    "msg": "",
    "tags": [
     {
      "key": "http.response.status_code",
      "value": "403"
     }
    ],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 1,
    "status": "error",
    "statusMessage": "",
    "duration": 168000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000030000000",
    "name": "GenerateThumbnail",
    "spanKind": "client",
    "service": "recording-api",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000005",
    "parentSpanId": "0000000000000001",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 122000,
    "events": [],
    "links": []
   },
   {
    "timeStamp": "1791792000032000000",
    "name": "generate_thumbnail",
    "spanKind": "server",
    "service": "thumbnail-worker",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000006",
    "parentSpanId": "0000000000000005",
    "msg": "",
    "tags": [],
    "serviceTags": [
     {
      "key": "service.version",
      "value": "1.1.0"
     }
    ],
    "childSpanCount": 0,
    "statusCode": 2,
    "status": "ok",
    "statusMessage": "",
    "duration": 118000,
    "events": [],
    "links": []
   }
  ],
  "logs": [
   {
    "timeUnixNano": "1791792000003000000",
    "recordId": "log-001",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000001",
    "severity": "INFO",
    "service": "",
    "message": "Recording request accepted",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000026000000",
    "recordId": "log-002",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Upload started",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000035000000",
    "recordId": "log-003",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Preparing metadata",
    "attributes": {
     "task.id": "metadata"
    }
   },
   {
    "timeUnixNano": "1791792000041000000",
    "recordId": "log-004",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000003",
    "severity": "INFO",
    "service": "",
    "message": "Sending object to storage",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000205000000",
    "recordId": "log-005",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000004",
    "severity": "ERROR",
    "service": "",
    "message": "AccessDenied: bucket policy rejected the request",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000211000000",
    "recordId": "log-006",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000003",
    "severity": "ERROR",
    "service": "",
    "message": "Storage returned HTTP 403",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000215000000",
    "recordId": "log-007",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000002",
    "severity": "ERROR",
    "service": "",
    "message": "Upload failed: AccessDenied",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000218000000",
    "recordId": "log-008",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000002",
    "severity": "ERROR",
    "service": "",
    "message": "Could not persist recording",
    "attributes": {
     "task.id": "upload"
    }
   },
   {
    "timeUnixNano": "1791792000080000000",
    "recordId": "log-009",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000002",
    "severity": "INFO",
    "service": "",
    "message": "Metadata prepared",
    "attributes": {
     "task.id": "metadata"
    }
   },
   {
    "timeUnixNano": "1791792000149000000",
    "recordId": "log-010",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000006",
    "severity": "INFO",
    "service": "",
    "message": "Thumbnail generated",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000235000000",
    "recordId": "log-011",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "0000000000000001",
    "severity": "WARN",
    "service": "",
    "message": "Recording was not published",
    "attributes": {}
   },
   {
    "timeUnixNano": "1791792000237000000",
    "recordId": "log-012",
    "traceId": "7a4c9d02000000000000000000000008",
    "spanId": "",
    "severity": "WARN",
    "service": "",
    "message": "Trace-level audit event: publication incomplete",
    "attributes": {}
   }
  ]
 }
}
