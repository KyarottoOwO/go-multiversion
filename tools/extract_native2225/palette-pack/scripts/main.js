import { world, system, BlockTypes, BlockStates, BlockPermutation, ItemTypes, ItemStack } from '@minecraft/server';
import { itemNames } from './items.js';

function* extract() {
  const types = BlockTypes.getAll().map(t => t.id).sort();
  const states = new Map();
  for (const state of BlockStates.getAll()) {
    console.warn('NATIVE2225_DOMAIN '+JSON.stringify({id:state.id,type:typeof state.validValues,values:state.validValues,keys:Object.getOwnPropertyNames(state)}));
    if (!state.validValues) { console.warn('NATIVE2225_BAD_STATE '+JSON.stringify(state)+' prototype='+Object.getOwnPropertyNames(Object.getPrototypeOf(state))); continue; }
    states.set(state.id,Array.from(state.validValues));
  }
  console.warn('NATIVE2225_BEGIN '+JSON.stringify({blockTypes:types.length,stateTypes:states.size}));
  const structure = world.structureManager.createEmpty('native2225:palette', {x:32,y:64,z:32});
  let count = 0, duplicates = 0, failures = [];
  const seen = new Set();
  for (const name of types) {
    let base;
    try { base = BlockPermutation.resolve(name); } catch(e) { failures.push({name,error:String(e)}); continue; }
    console.warn('NATIVE2225_BLOCK_ITEM '+JSON.stringify({name,item:base.getItemStack()?.typeId??null,tags:base.getTags()}));
    const keys = Object.keys(base.getAllStates()).sort();
    let combinations = [{}];
    for (const key of keys) {
      const values = states.get(key);
      if (!values) throw new Error('Missing state domain '+key);
      combinations = combinations.flatMap(c => values.map(v => ({...c,[key]:v})));
      if (combinations.length>65536) throw new Error('Unbounded product '+name);
    }
    let accepted = 0;
    for (const combination of combinations) {
      let permutation;
      try { permutation = BlockPermutation.resolve(name, combination); }
      catch(e) { failures.push({name,states:combination,error:String(e)}); continue; }
      const actual = permutation.getAllStates();
      const ordered = Object.fromEntries(Object.keys(actual).sort().map(k=>[k,actual[k]]));
      const key = JSON.stringify([permutation.type.id, ordered]);
      if (seen.has(key)) { duplicates++; continue; }
      if (count>=65536) throw new Error('Palette exceeded structure capacity');
      seen.add(key);
      const location = {x:count%32,y:Math.floor(count/1024),z:Math.floor(count/32)%32};
      try { structure.setBlockPermutation(location, permutation); }
      catch(e) { failures.push({name,states:ordered,stage:'structure',error:String(e)}); seen.delete(key); continue; }
      console.warn('NATIVE2225_STATE '+JSON.stringify({index:count,name:permutation.type.id,states:ordered}));
      count++; accepted++;
      if (count%128===0) yield;
    }
    console.warn('NATIVE2225_TYPE '+JSON.stringify({name,attempted:combinations.length,accepted}));
    yield;
  }
  structure.saveToWorld();
  const candidate = world.structureManager.get('native2225:probe');
  if (candidate) {
    for (let x=0;x<candidate.size.x;x++) {
      const p=candidate.getBlockPermutation({x,y:0,z:0});
      console.warn('NATIVE2225_CANDIDATE '+JSON.stringify({x,name:p?.type.id,states:p?.getAllStates()}));
    }
    candidate.saveAs('native2225:validated_probe');
  }
  const itemTypes = ItemTypes.getAll();
  const missingItems = [];
  for (const name of itemNames) {
    try {
      const stack = new ItemStack(name);
      console.warn('NATIVE2225_ITEM_TAGS '+JSON.stringify({name,tags:stack.getTags()}));
    } catch(e) { missingItems.push({name,error:String(e)}); }
    yield;
  }
  console.warn('NATIVE2225_DONE '+JSON.stringify({blockTypes:types.length,states:count,duplicates,failures,scriptItemTypes:itemTypes.length,missingItems}));
}

world.afterEvents.worldLoad.subscribe(()=>system.runJob(extract()));
