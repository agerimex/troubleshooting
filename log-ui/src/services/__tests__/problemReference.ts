// Generated from troubleshooting-ui-agent/fixtures/ui-reference.json (reports "failed" and "logonly"),
// converted to the API shapes of the trace report. Presentation data, not a population.
import { type Span } from '@/types/filter'
import { type LogRecord } from '@/types/report'

export const referenceReports: Record<string, { traceId: string, entrySpanId: string, spans: Span[], logs: LogRecord[] }> = {
  "failed": {
    "traceId": "be51eb0c050200000000000000000001",
    "entrySpanId": "0000000000000002",
    "spans": [
      {
        "timeStamp": "1791709092000000000",
        "name": "publish_recording",
        "service": "recording-api",
        "traceId": "be51eb0c050200000000000000000001",
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
        "timeStamp": "1791709092025000000",
        "name": "upload_recording",
        "service": "recording-worker",
        "traceId": "be51eb0c050200000000000000000001",
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
        "timeStamp": "1791709092040000000",
        "name": "storage.PutObject",
        "service": "recording-worker",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000004",
        "parentSpanId": "0000000000000002",
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
        "duration": 170000,
        "events": [],
        "links": []
      },
      {
        "timeStamp": "1791709092030000000",
        "name": "create_thumbnail",
        "service": "recording-worker",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000003",
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
        "duration": 120000,
        "events": [],
        "links": []
      }
    ],
    "logs": [
      {
        "timeUnixNano": "1791709092001000000",
        "recordId": "be51eb0c050200000000000000000001-r1",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000001",
        "severity": "INFO",
        "service": "recording-api",
        "message": "Publish request received",
        "attributes": {
          "operation.name": "publish_recording",
          "span.kind": "SERVER"
        }
      },
      {
        "timeUnixNano": "1791709092238000000",
        "recordId": "be51eb0c050200000000000000000001-r2",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000001",
        "severity": "ERROR",
        "service": "recording-api",
        "message": "Recording publish failed",
        "attributes": {
          "operation.name": "publish_recording",
          "span.kind": "SERVER",
          "error.code": "PublishFailed"
        }
      },
      {
        "timeUnixNano": "1791709092026000000",
        "recordId": "be51eb0c050200000000000000000001-u1",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000002",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "Upload scheduled",
        "attributes": {
          "operation.name": "upload_recording",
          "span.kind": "CONSUMER"
        }
      },
      {
        "timeUnixNano": "1791709092042000000",
        "recordId": "be51eb0c050200000000000000000001-u2",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000002",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "Transfer started",
        "attributes": {
          "operation.name": "upload_recording",
          "span.kind": "CONSUMER",
          "task.id": "transfer"
        }
      },
      {
        "timeUnixNano": "1791709092050000000",
        "recordId": "be51eb0c050200000000000000000001-u3",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000002",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "Checksum calculation started",
        "attributes": {
          "operation.name": "upload_recording",
          "span.kind": "CONSUMER",
          "task.id": "checksum"
        }
      },
      {
        "timeUnixNano": "1791709092090000000",
        "recordId": "be51eb0c050200000000000000000001-u4",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000002",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "Checksum calculated",
        "attributes": {
          "operation.name": "upload_recording",
          "span.kind": "CONSUMER",
          "task.id": "checksum"
        }
      },
      {
        "timeUnixNano": "1791709092120000000",
        "recordId": "be51eb0c050200000000000000000001-u5",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000002",
        "severity": "WARN",
        "service": "recording-worker",
        "message": "Upload retry 1 of 3",
        "attributes": {
          "operation.name": "upload_recording",
          "span.kind": "CONSUMER",
          "task.id": "transfer"
        }
      },
      {
        "timeUnixNano": "1791709092205000000",
        "recordId": "be51eb0c050200000000000000000001-u6",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000002",
        "severity": "ERROR",
        "service": "recording-worker",
        "message": "Storage returned 403 AccessDenied",
        "attributes": {
          "operation.name": "upload_recording",
          "span.kind": "CONSUMER",
          "task.id": "transfer",
          "error.code": "AccessDenied"
        }
      },
      {
        "timeUnixNano": "1791709092041000000",
        "recordId": "be51eb0c050200000000000000000001-s1",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000004",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "PutObject request sent",
        "attributes": {
          "operation.name": "storage.PutObject",
          "span.kind": "CLIENT"
        }
      },
      {
        "timeUnixNano": "1791709092209000000",
        "recordId": "be51eb0c050200000000000000000001-s2",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000004",
        "severity": "ERROR",
        "service": "recording-worker",
        "message": "HTTP 403: access denied for object upload",
        "attributes": {
          "operation.name": "storage.PutObject",
          "span.kind": "CLIENT",
          "error.code": "AccessDenied"
        }
      },
      {
        "timeUnixNano": "1791709092031000000",
        "recordId": "be51eb0c050200000000000000000001-t1",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000003",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "Thumbnail generation started",
        "attributes": {
          "operation.name": "create_thumbnail",
          "span.kind": "INTERNAL",
          "task.id": "thumbnail"
        }
      },
      {
        "timeUnixNano": "1791709092149000000",
        "recordId": "be51eb0c050200000000000000000001-t2",
        "traceId": "be51eb0c050200000000000000000001",
        "spanId": "0000000000000003",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "Thumbnail ready",
        "attributes": {
          "operation.name": "create_thumbnail",
          "span.kind": "INTERNAL",
          "task.id": "thumbnail"
        }
      }
    ]
  },
  "logonly": {
    "traceId": "be51eb0c050200000000000000000002",
    "entrySpanId": "0000000000000002",
    "spans": [
      {
        "timeStamp": "1791708423000000000",
        "name": "publish_recording",
        "service": "recording-api",
        "traceId": "be51eb0c050200000000000000000002",
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
        "duration": 220000,
        "events": [],
        "links": []
      },
      {
        "timeStamp": "1791708423025000000",
        "name": "upload_recording",
        "service": "recording-worker",
        "traceId": "be51eb0c050200000000000000000002",
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
        "duration": 190000,
        "events": [],
        "links": []
      },
      {
        "timeStamp": "1791708423040000000",
        "name": "storage.PutObject",
        "service": "recording-worker",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000004",
        "parentSpanId": "0000000000000002",
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
        "duration": 168000,
        "events": [],
        "links": []
      },
      {
        "timeStamp": "1791708423030000000",
        "name": "create_thumbnail",
        "service": "recording-worker",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000003",
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
        "duration": 110000,
        "events": [],
        "links": []
      }
    ],
    "logs": [
      {
        "timeUnixNano": "1791708423001000000",
        "recordId": "be51eb0c050200000000000000000002-r1",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000001",
        "severity": "INFO",
        "service": "recording-api",
        "message": "Publish request received",
        "attributes": {
          "operation.name": "publish_recording",
          "span.kind": "SERVER"
        }
      },
      {
        "timeUnixNano": "1791708423218000000",
        "recordId": "be51eb0c050200000000000000000002-r2",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000001",
        "severity": "INFO",
        "service": "recording-api",
        "message": "Publish handler completed",
        "attributes": {
          "operation.name": "publish_recording",
          "span.kind": "SERVER"
        }
      },
      {
        "timeUnixNano": "1791708423026000000",
        "recordId": "be51eb0c050200000000000000000002-u1",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000002",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "Upload scheduled",
        "attributes": {
          "operation.name": "upload_recording",
          "span.kind": "CONSUMER"
        }
      },
      {
        "timeUnixNano": "1791708423042000000",
        "recordId": "be51eb0c050200000000000000000002-u2",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000002",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "Transfer started",
        "attributes": {
          "operation.name": "upload_recording",
          "span.kind": "CONSUMER",
          "task.id": "transfer"
        }
      },
      {
        "timeUnixNano": "1791708423050000000",
        "recordId": "be51eb0c050200000000000000000002-u3",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000002",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "Checksum calculation started",
        "attributes": {
          "operation.name": "upload_recording",
          "span.kind": "CONSUMER",
          "task.id": "checksum"
        }
      },
      {
        "timeUnixNano": "1791708423090000000",
        "recordId": "be51eb0c050200000000000000000002-u4",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000002",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "Checksum calculated",
        "attributes": {
          "operation.name": "upload_recording",
          "span.kind": "CONSUMER",
          "task.id": "checksum"
        }
      },
      {
        "timeUnixNano": "1791708423120000000",
        "recordId": "be51eb0c050200000000000000000002-u5",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000002",
        "severity": "WARN",
        "service": "recording-worker",
        "message": "Upload retry 1 of 3",
        "attributes": {
          "operation.name": "upload_recording",
          "span.kind": "CONSUMER",
          "task.id": "transfer"
        }
      },
      {
        "timeUnixNano": "1791708423205000000",
        "recordId": "be51eb0c050200000000000000000002-u6",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000002",
        "severity": "ERROR",
        "service": "recording-worker",
        "message": "Storage returned 403 AccessDenied",
        "attributes": {
          "operation.name": "upload_recording",
          "span.kind": "CONSUMER",
          "task.id": "transfer",
          "error.code": "AccessDenied"
        }
      },
      {
        "timeUnixNano": "1791708423041000000",
        "recordId": "be51eb0c050200000000000000000002-s1",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000004",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "PutObject request sent",
        "attributes": {
          "operation.name": "storage.PutObject",
          "span.kind": "CLIENT"
        }
      },
      {
        "timeUnixNano": "1791708423207000000",
        "recordId": "be51eb0c050200000000000000000002-s2",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000004",
        "severity": "ERROR",
        "service": "recording-worker",
        "message": "HTTP 403: access denied for object upload",
        "attributes": {
          "operation.name": "storage.PutObject",
          "span.kind": "CLIENT",
          "error.code": "AccessDenied"
        }
      },
      {
        "timeUnixNano": "1791708423031000000",
        "recordId": "be51eb0c050200000000000000000002-t1",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000003",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "Thumbnail generation started",
        "attributes": {
          "operation.name": "create_thumbnail",
          "span.kind": "INTERNAL",
          "task.id": "thumbnail"
        }
      },
      {
        "timeUnixNano": "1791708423139000000",
        "recordId": "be51eb0c050200000000000000000002-t2",
        "traceId": "be51eb0c050200000000000000000002",
        "spanId": "0000000000000003",
        "severity": "INFO",
        "service": "recording-worker",
        "message": "Thumbnail ready",
        "attributes": {
          "operation.name": "create_thumbnail",
          "span.kind": "INTERNAL",
          "task.id": "thumbnail"
        }
      }
    ]
  }
}
