// Group detail forms share the existing authenticated APIs.
let studyPlans = [];
let editingPlanID = null;
async function socialRequest(url, options = {}) {
    const response = await fetch(url, options);
    const text = await response.text();
    let data;
    try { data = text ? JSON.parse(text) : null; } catch (_) { data = null; }
    if (!response.ok) throw new Error(data?.error || text || 'Request failed');
    return data;
}
function closePlanEditor() {
    const modal = document.getElementById('addStudyPlanModal');
    modal.classList.add('hidden'); modal.classList.remove('flex');
    document.getElementById('createStudyPlanBtn').focus();
}
async function loadStudyPlan() {
    const container = document.getElementById('studyPlanContent');
    try {
        studyPlans = await socialRequest(`/api/study-plans/list?group_id=${currentGroupId}`) || [];
        container.replaceChildren();
        if (!studyPlans.length) { container.textContent = 'No reading schedule yet. A group admin can add the first week.'; return; }
        for (const plan of studyPlans) {
            const card = document.createElement('article');
            card.className = 'group-section p-4 rounded-lg mb-3';
            const title = document.createElement('h3'); title.className = 'font-semibold';
            title.textContent = `Week ${plan.week_number}: ${plan.book} ${plan.start_chapter}–${plan.end_chapter}`;
            const dates = document.createElement('p'); dates.className = 'text-sm my-2';
            dates.textContent = `${plan.start_date.slice(0,10)} – ${plan.end_date.slice(0,10)}`;
            const description = document.createElement('p'); description.className = 'whitespace-pre-wrap mb-3'; description.textContent = plan.description;
            const actions = document.createElement('div'); actions.className = 'flex flex-wrap gap-3';
            const link = document.createElement('a'); link.className = 'underline'; link.textContent = 'Start reading';
            link.href = '/?' + new URLSearchParams({book:plan.book,chapter:plan.start_chapter});
            actions.append(link);
            if (currentUserIsAdmin) {
                for (const [label, action] of [['Edit', () => openPlanEditor(plan)], ['Delete', () => deletePlan(plan)]]) {
                    const button = document.createElement('button'); button.type = 'button'; button.className = 'underline'; button.textContent = label; button.onclick = action; actions.append(button);
                }
            }
            card.append(title,dates,description,actions); container.append(card);
        }
    } catch (error) { container.textContent = error.message; }
}
function openPlanEditor(plan = null) {
    editingPlanID = plan?.id || null;
    document.getElementById('addStudyPlanForm').reset();
    const values = {planWeekNumber: plan?.week_number || Math.max(0,...studyPlans.map(p=>p.week_number))+1,
        planStartDate:plan?.start_date?.slice(0,10)||'',planEndDate:plan?.end_date?.slice(0,10)||'',
        planBook:plan?.book||'',planStartChapter:plan?.start_chapter||1,planEndChapter:plan?.end_chapter||1,planDescription:plan?.description||''};
    for (const [id,value] of Object.entries(values)) document.getElementById(id).value=value;
    const modal=document.getElementById('addStudyPlanModal');
    modal.querySelector('h2').textContent=plan?'Edit Study Plan Week':'Add Study Plan Week';
    modal.querySelector('[type="submit"]').textContent='Save week';
    modal.classList.remove('hidden'); modal.classList.add('flex');
    document.getElementById('planWeekNumber').focus();
}
async function deletePlan(plan) {
    if (!confirm(`Delete week ${plan.week_number} from this study plan?`)) return;
    try {
        await socialRequest('/api/study-plans/delete',{method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:plan.id,group_id:currentGroupId})});
        await loadStudyPlan();
    } catch(error) { showMessage('Unable to delete week',error.message); }
}
document.getElementById('createStudyPlanBtn').addEventListener('click',()=>openPlanEditor());
document.getElementById('cancelPlanBtn').addEventListener('click',closePlanEditor);
document.getElementById('addStudyPlanForm').addEventListener('submit',async event=>{
    event.preventDefault(); const button=event.submitter; button.disabled=true;
    const value=id=>document.getElementById(id).value;
    const plan={id:editingPlanID,group_id:currentGroupId,week_number:Number(value('planWeekNumber')),start_date:value('planStartDate'),end_date:value('planEndDate'),book:value('planBook').trim(),start_chapter:Number(value('planStartChapter')),end_chapter:Number(value('planEndChapter')),description:value('planDescription').trim()};
    try {
        await socialRequest(`/api/study-plans/${editingPlanID?'update':'create'}`,{method:editingPlanID?'PUT':'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(plan)});
        closePlanEditor(); await loadStudyPlan();
    } catch(error) { showMessage('Unable to save week',error.message); }
    finally { button.disabled=false; }
});
document.getElementById('inviteMemberForm').addEventListener('submit',async event=>{
    event.preventDefault(); const button=event.submitter; button.disabled=true;
    try {
        await socialRequest(`/api/groups/${currentGroupId}/invite`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:document.getElementById('inviteEmail').value.trim()})});
        document.getElementById('inviteMemberModal').classList.add('hidden');
        document.getElementById('inviteMemberModal').classList.remove('flex');
        event.target.reset(); showMessage('Invitation created','They can accept the invitation on their Study Groups page when signed in with that email address.');
    } catch(error) { showMessage('Unable to invite member',error.message); }
    finally { button.disabled=false; }
});
document.addEventListener('keydown',event=>{if(event.key==='Escape')closePlanEditor();});

let editingPrayerID = null;
async function editPrayerRequest(id) {
    try {
        const prayer=await socialRequest(`/api/prayers/${id}`);
        openAddPrayerModal(); editingPrayerID=id;
        document.querySelector('#addPrayerModal h2').textContent='Edit Prayer Request';
        document.getElementById('prayerTitle').value=prayer.title;
        document.getElementById('prayerContent').value=prayer.content;
    } catch(error) { showMessage('Unable to edit request',error.message); }
}
async function deletePrayerRequest(id) {
    if(!confirm('Delete this prayer request and its encouragements?'))return;
    try {await socialRequest(`/api/prayers/${id}/delete`,{method:'DELETE'});await loadPrayers();}
    catch(error){showMessage('Unable to delete request',error.message);}
}
let groupSocket;
let groupReconnect;
function connectGroupUpdates() {
    if(!currentUserId)return;
    groupSocket=new WebSocket(`${location.protocol==='https:'?'wss:':'ws:'}//${location.host}/ws`);
    groupSocket.onmessage=event=>{
        const message=JSON.parse(event.data);
        if(message.data?.group_id!==currentGroupId)return;
        let notice=document.getElementById('groupUpdates');
        if(!notice){
            notice=document.createElement('button'); notice.id='groupUpdates'; notice.className='w-full p-3 mb-3 rounded-lg border';
            notice.textContent='New group activity — refresh when you’re ready';
            notice.onclick=async()=>{
                await loadMembers();
                if(!document.getElementById('prayersTab').classList.contains('hidden'))await loadPrayers();
                if(!document.getElementById('studyplanTab').classList.contains('hidden'))await loadStudyPlan();
                notice.remove();
            };
            document.querySelector('.tab-content').parentElement.prepend(notice);
        }
    };
    groupSocket.onclose=()=>{clearTimeout(groupReconnect);groupReconnect=setTimeout(connectGroupUpdates,5000);};
}
async function deleteEncouragement(id,prayerID) {
    if(!confirm('Delete your encouragement?'))return;
    try {await socialRequest(`/api/prayer-comments/delete?commentId=${id}`,{method:'DELETE'});await loadPrayerEncouragements(prayerID);}
    catch(error){showMessage('Unable to delete encouragement',error.message);}
}
