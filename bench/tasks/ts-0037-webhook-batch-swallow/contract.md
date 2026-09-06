# Contract: bad events are reported, not fatal

<!-- canary: 79096aee991b23d5 -->
REQUEST: sha256:d51bcb3dabe7a25a1c5fedde97b3f9a1ec3e622a479861c95b210bb179f73fc1
IN: src/**
OUT: test/**, fixtures/**

- [ ] G1: the batch survives a bad event and the visible suite is green
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R4 "The new case in test/dispatch.test.ts fails"
- [ ] G2: bad events appear in failed with their id, position and the renderer's reason
    CHECK: node --input-type=module -e 'const d=await import("./src/dispatch.ts");const r=await import("./src/render.ts");const evts=[{id:"a1",kind:"invoice.paid",data:{email:"x@y.example",amount:"1.00"}},{id:"a2",kind:"invoice.paid",data:{email:"x@y.example",amount:"nope"}},{id:"a3",kind:"refund.issued",data:{email:"x@y.example",amount:"2.50"}}];const t={sent:[],async send(p){this.sent.push(p)}};const res=await d.dispatchAll(evts,t);let why="";try{r.renderEvent(evts[1])}catch(e){why=e.message}console.log("failed",JSON.stringify(res.failed.map(f=>[f.id,Number(f.position),String(f.reason).includes(why)])),"sent",JSON.stringify(res.sent.map(p=>p.id)))'
    EXPECT: /^failed \[\["a2",2,true\]\] sent \["a1","a3"\]$/m
    FROM: R2 "every bad event is reported in the result's failed list with the event's id, its position in the batch"
- [ ] G3: a transport failure still stops the dispatch
    CHECK: node --input-type=module -e 'const d=await import("./src/dispatch.ts");const r=await import("./src/render.ts");const t={async send(){throw new r.TransportError("endpoint down")}};try{await d.dispatchAll([{id:"b1",kind:"invoice.paid",data:{email:"x@y.example",amount:"1.00"}}],t);console.log("transport-error swallowed")}catch(e){console.log(e instanceof r.TransportError?"transport-error raised":"other-error")}'
    EXPECT: /^transport-error raised$/m
    FROM: R3 "must still stop the dispatch as it does today"
    RED: none
