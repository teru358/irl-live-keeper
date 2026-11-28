export function cameraHeading(camera,t){return t('settings_camera')+' '+camera.id.slice(3)+(camera.name?(t('settings_camera')==='カメラ'?'（'+camera.name+'）':' ('+camera.name+')'):'');}
