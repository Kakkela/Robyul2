package idols

import (
	"fmt"
	"image"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Seklfreak/Robyul2/cache"
	"github.com/Seklfreak/Robyul2/helpers"
	"github.com/Seklfreak/Robyul2/models"
	"github.com/bwmarrin/discordgo"
	humanize "github.com/dustin/go-humanize"
	"github.com/globalsign/mgo/bson"
)

const (
	ALL_IDOLS_CACHE_KEY = "allidols"
)

// holds all available idols
var allIdols []*Idol
var activeIdols []*Idol
var allIdolsMutex sync.RWMutex

////////////////////
//  Idol Methods  //
////////////////////

// GetRandomImage returns a random idol image
func (i *Idol) GetRandomImage() image.Image {

	imageIndex := rand.Intn(len(i.Images))
	imgBytes := i.Images[imageIndex].GetImgBytes()
	img, _, err := helpers.DecodeImageBytes(imgBytes)
	helpers.Relax(err)
	return img
}

// GetResizedRandomImage returns a random image that has been resized
func (i *Idol) GetResizedRandomImage(resize int) image.Image {

	imageIndex := rand.Intn(len(i.Images))
	imgBytes := i.Images[imageIndex].GetResizeImgBytes(resize)
	img, _, err := helpers.DecodeImageBytes(imgBytes)
	helpers.Relax(err)
	return img
}

// GetAllIdols getter for all idols
func GetAllIdols() []*Idol {
	allIdolsMutex.RLock()
	defer allIdolsMutex.RUnlock()
	return allIdols
}

// GetActiveIdols will return only active idols and idols with images
func GetActiveIdols() []*Idol {
	allIdolsMutex.RLock()
	defer allIdolsMutex.RUnlock()
	return activeIdols
}

////////////////////////
//  Public Functions  //
////////////////////////

// GetMatchingIdolById will get a matching idol based on the given id, will return nil if non is found
func GetMatchingIdolById(id bson.ObjectId) *Idol {
	var matchingIdol *Idol

	for _, idol := range GetAllIdols() {
		if idol.ID == id {
			matchingIdol = idol
			break
		}
	}

	return matchingIdol
}

// GetMatchingIdolAndGroup will do a loose comparison of the name and group passed to the ones that already exist
//  1st return is true if group exists
//  2nd return is true if idol exists in the group
//  3rd will be a reference to the matching idol
func GetMatchingIdolAndGroup(searchGroup, searchName string, activeOnly bool) (bool, bool, *Idol) {
	groupMatch := false
	nameMatch := false
	var matchingIdol *Idol

	// find a matching group
	groupMatch, realMatchingGroupName := GetMatchingGroup(searchGroup, activeOnly)

	// if no matching group was found, just return 0 values
	if !groupMatch {
		return false, false, nil
	}

	// find matching idol in the matching group
	var idolsToCheck []*Idol
	if activeOnly {
		idolsToCheck = GetActiveIdols()
	} else {
		idolsToCheck = GetAllIdols()
	}

	allIdolsMutex.RLock()
IdolLoop:
	for _, idol := range idolsToCheck {

		if idol.GroupName != realMatchingGroupName {
			continue
		}

		if alphaNumericCompare(idol.Name, searchName) {
			nameMatch = true
			matchingIdol = idol
			break
		}

		// if the given name doesn't match, check the aliases
		for _, alias := range idol.NameAliases {
			if alphaNumericCompare(alias, searchName) {
				nameMatch = true
				matchingIdol = idol
				break IdolLoop
			}
		}
	}
	allIdolsMutex.RUnlock()

	return groupMatch, nameMatch, matchingIdol
}

// getMatchingGroup will do a loose comparison of the group name to see if it exists
// return 1: if a matching group exists
// return 2: what the real group name is
func GetMatchingGroup(searchGroup string, activeOnly bool) (bool, string) {

	allGroupsMap := make(map[string]bool)
	if activeOnly {
		for _, idol := range GetActiveIdols() {
			allGroupsMap[idol.GroupName] = true
		}
	} else {

		for _, idol := range GetAllIdols() {
			allGroupsMap[idol.GroupName] = true
		}
	}

	groupAliases := getGroupAliases()

	// check if the group suggested matches a current group. do loose comparison
	for currentGroup := range allGroupsMap {

		// if groups match, set the suggested group to the current group
		if alphaNumericCompare(currentGroup, searchGroup) {
			return true, currentGroup
		}

		// if this group has any aliases check if the group we're
		//   searching for matches one of the aliases
		for aliasGroup, aliases := range groupAliases {
			if !alphaNumericCompare(aliasGroup, currentGroup) {
				continue
			}

			for _, alias := range aliases {
				if alphaNumericCompare(alias, searchGroup) {
					return true, currentGroup
				}
			}
		}
	}

	return false, ""
}

/////////////////////////
//  Private Functions  //
/////////////////////////

// startCacheRefreshLoop will refresh the image cache for idols
func startCacheRefreshLoop() {
	log().Info("Starting refresh idol image cache loop")
	go func() {
		defer helpers.Recover()

		for {
			time.Sleep(time.Hour * 12)

			log().Info("Refreshing image cache...")
			refreshIdols(true)

			log().Info("Idol image cache has been refresh")
		}
	}()
}

// refreshIdolsFromOld refreshes the idols
//   initially called when bot starts but is also safe to call while bot is running if necessary
// DEPRECATED - refreshes idols from old idols table.
func refreshIdolsFromOld(skipCache bool) {

	if !skipCache {

		// attempt to get redis cache, return if its successful
		var tempAllIdols []*Idol
		err := getModuleCache(ALL_IDOLS_CACHE_KEY, &tempAllIdols)
		if err == nil {
			setAllIdols(tempAllIdols)
			log().Info("Idols loaded from cache")
			return
		}

		log().Info("Idols loading from mongodb. Cache not set or expired.")
	}

	var idolEntries []models.OldIdolEntry
	err := helpers.MDbIter(helpers.MdbCollection(models.OldIdolsTable).Find(bson.M{})).All(&idolEntries)
	helpers.Relax(err)

	log().Infof("Loading idols. Total image records: %d", len(idolEntries))

	var tempAllIdols []*Idol

	// run limited amount of goroutines at the same time
	mux := new(sync.Mutex)
	sem := make(chan bool, 50)
	for _, idolEntry := range idolEntries {
		sem <- true
		go func(idolEntry models.OldIdolEntry) {
			defer func() { <-sem }()
			defer helpers.Recover()

			newIdol := makeIdolFromOldIdolEntry(idolEntry)

			mux.Lock()
			defer mux.Unlock()

			// if the idol already exists, then just add this picture to the image array for the idol
			for _, currentIdol := range tempAllIdols {
				if currentIdol.NameAndGroup == newIdol.NameAndGroup {
					currentIdol.Images = append(currentIdol.Images, newIdol.Images[0])
					return
				}
			}
			tempAllIdols = append(tempAllIdols, &newIdol)
		}(idolEntry)
	}
	for i := 0; i < cap(sem); i++ {
		sem <- true
	}

	log().Info("Amount of idols loaded: ", len(tempAllIdols))
	setAllIdols(tempAllIdols)

	// cache all idols
	if len(GetAllIdols()) > 0 {
		err = setModuleCache(ALL_IDOLS_CACHE_KEY, GetAllIdols(), time.Hour*24*7)
		helpers.RelaxLog(err)
	}
}

// refreshIdols refreshes the idols
//   initially called when bot starts but is also safe to call while bot is running if necessary
func refreshIdols(skipCache bool) {

	if !skipCache {

		// attempt to get redis cache, return if its successful
		var tempAllIdols []*Idol
		err := getModuleCache(ALL_IDOLS_CACHE_KEY, &tempAllIdols)
		if err == nil {
			setAllIdols(tempAllIdols)
			log().Info("Idols loaded from cache")
			return
		}

		log().Info("Idols loading from mongodb. Cache not set or expired.")
	}

	var idolEntries []models.IdolEntry
	err := helpers.MDbIter(helpers.MdbCollection(models.IdolTable).Find(bson.M{})).All(&idolEntries)
	helpers.Relax(err)

	// confirm records were retrieved
	if len(idolEntries) == 0 {
		log().Errorln("Refreshing idols failed. Table is empty likely because migration hasn't been run yet.")
		return
	}

	log().Infof("Loading idols. Total idol records: %d", len(idolEntries))

	var tempAllIdols []*Idol

	// run limited amount of goroutines at the same time
	for _, idolEntry := range idolEntries {
		// create new idol from the idol entry in mongo
		newIdol := makeIdolFromIdolEntry(idolEntry)
		tempAllIdols = append(tempAllIdols, &newIdol)
	}

	log().Info("Amount of idols loaded: ", len(tempAllIdols))
	setAllIdols(tempAllIdols)

	// cache all idols
	if len(GetAllIdols()) > 0 {
		err = setModuleCache(ALL_IDOLS_CACHE_KEY, GetAllIdols(), time.Hour*24*7)
		helpers.RelaxLog(err)
	}
}

// makeIdolFromIdolEntry takes a mdb idol entry and makes a idol
func makeIdolFromIdolEntry(entry models.IdolEntry) Idol {
	// create new idol from the idol entry in mongo
	newIdol := Idol{
		ID:           entry.ID,
		Name:         entry.Name,
		GroupName:    entry.GroupName,
		NameAndGroup: entry.Name + entry.GroupName,
		NameAliases:  entry.NameAliases,
		Gender:       entry.Gender,
	}

	// convert idol entry images
	for _, idolImageEntry := range entry.Images {
		newIdol.Images = append(newIdol.Images, IdolImage{
			HashString: idolImageEntry.HashString,
			ObjectName: idolImageEntry.ObjectName,
		})
	}

	return newIdol
}

// makeIdolFromOldIdolEntry takes a mdb idol entry and makes a idol
func makeIdolFromOldIdolEntry(entry models.OldIdolEntry) Idol {
	iImage := IdolImage{
		ObjectName: entry.ObjectName,
	}

	// get image hash string
	img, _, err := helpers.DecodeImageBytes(iImage.GetImgBytes())
	helpers.Relax(err)
	imgHash, err := helpers.GetImageHashString(img)
	helpers.Relax(err)
	iImage.HashString = imgHash

	newIdol := Idol{
		Name:         entry.Name,
		GroupName:    entry.GroupName,
		Gender:       entry.Gender,
		NameAndGroup: entry.Name + entry.GroupName,
		Images:       []IdolImage{iImage},
	}
	return newIdol
}

// updateGroupInfo if a target group is found, this will update the group name
//  for all members
func updateGroupInfo(msg *discordgo.Message, content string) {
	cache.GetSession().ChannelTyping(msg.ChannelID)

	contentArgs, err := helpers.ToArgv(content)
	if err != nil {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.invalid"))
		return
	}
	contentArgs = contentArgs[1:]

	// confirm amount of args
	if len(contentArgs) != 2 {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.invalid"))
		return
	}

	targetGroup := contentArgs[0]
	newGroup := contentArgs[1]

	// confirm target group exists
	if matched, realGroupName := GetMatchingGroup(targetGroup, true); !matched {
		helpers.SendMessage(msg.ChannelID, "No group found with that exact name.")
		return
	} else {
		targetGroup = realGroupName
	}

	// update all idols in the target group
	var idolsUpdated int
	for _, idol := range GetAllIdols() {
		if idol.GroupName == targetGroup {

			recordsUpdated := updateIdolInfo(idol.GroupName, idol.Name, newGroup, idol.Name, idol.Gender)
			if recordsUpdated != 0 {
				idolsUpdated++
			}
			helpers.SendMessage(msg.ChannelID, fmt.Sprintf("Updated Idol: **%s** %s => **%s** %s", targetGroup, idol.Name, newGroup, idol.Name))

			// sleep so mongo doesn't get flooded with update reqeusts
			time.Sleep(time.Second / 5)
		}
	}

	// check if an idol record was updated
	if idolsUpdated == 0 {
		helpers.SendMessage(msg.ChannelID, "No Idols found in the given group.")
	} else {
		helpers.SendMessage(msg.ChannelID, fmt.Sprintf("Group Information updated. \nIdols Updated: %d", idolsUpdated))
	}
}

// updateIdolInfoFromMsg updates a idols group, name, and/or gender depending on args
func updateIdolInfoFromMsg(msg *discordgo.Message, content string) {
	cache.GetSession().ChannelTyping(msg.ChannelID)

	contentArgs, err := helpers.ToArgv(content)
	if err != nil {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.invalid"))
		return
	}
	contentArgs = contentArgs[1:]

	// confirm amount of args
	if len(contentArgs) < 5 {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.too-few"))
		return
	}

	// validate gender
	if contentArgs[4] != "boy" && contentArgs[4] != "girl" {
		helpers.SendMessage(msg.ChannelID, "Invalid gender. Gender must be exactly 'girl' or 'boy'. No information was updated.")
		return
	}

	targetGroup := contentArgs[0]
	targetName := contentArgs[1]
	newGroup := contentArgs[2]
	newName := contentArgs[3]
	newGender := contentArgs[4]

	// update idol
	recordsUpdated := updateIdolInfo(targetGroup, targetName, newGroup, newName, newGender)

	// check if an idol record was updated
	if recordsUpdated == 0 {
		helpers.SendMessage(msg.ChannelID, "No Idols found with that exact group and name.")
	} else {
		helpers.SendMessage(msg.ChannelID, fmt.Sprintf("Idol Information updated. \nOld: **%s** %s \nNew: **%s** %s", targetGroup, targetName, newGroup, newName))
	}
}

// updateIdolInfo updates a idols group, name, and gender depending on args
func updateIdolInfo(targetGroup, targetName, newGroup, newName, newGender string) int {

	// attempt to find a matching idol of the new group and name
	_, _, matchingIdol := GetMatchingIdolAndGroup(newGroup, newName, false)

	// GetMatchingIdolAndGroup does a lose alpha-numeric compare but we need
	// to do a stricter check in order to accomedate simply wanting to change
	// spacing or capitolization
	if matchingIdol != nil {
		if matchingIdol.GroupName != newGroup || matchingIdol.Name != newName {
			matchingIdol = nil
		}
	}

	recordsFound := 0

	// update idols in memory
	allIdols := GetAllIdols()
	allIdolsMutex.Lock()
	for idolIndex, targetIdol := range allIdols {
		if targetIdol.Name != targetName || targetIdol.GroupName != targetGroup {
			continue
		}
		recordsFound++

		// if a matching idol was is found, just assign the targets images to it and delete
		if matchingIdol != nil && (matchingIdol.Name != targetIdol.Name || matchingIdol.GroupName != targetIdol.GroupName) {

			matchingIdol.Images = append(matchingIdol.Images, targetIdol.Images...)
			allIdols = append(allIdols[:idolIndex], allIdols[idolIndex+1:]...)

		} else {

			// update targetIdol name and group
			targetIdol.Name = newName
			targetIdol.GroupName = newGroup
			targetIdol.Gender = newGender
		}
	}
	allIdolsMutex.Unlock()
	setAllIdols(allIdols)

	// update database
	var idolsToUpdate []models.IdolEntry
	err := helpers.MDbIter(helpers.MdbCollection(models.IdolTable).Find(bson.M{"groupname": targetGroup, "name": targetName})).All(&idolsToUpdate)
	helpers.Relax(err)

	targetIdolFound := false

	// if a matching idol was found then assign all the images to the target idol and delete the current
	if matchingIdol != nil {

		var targetIdol models.IdolEntry
		err := helpers.MdbOne(helpers.MdbCollection(models.IdolTable).Find(bson.M{"groupname": matchingIdol.GroupName, "name": matchingIdol.Name}), &targetIdol)
		helpers.Relax(err)

		// make sure mongo returned a valid target idol
		if targetIdol.GroupName != "" {
			targetIdolFound = true

			// assign all images to the target idol
			for _, idol := range idolsToUpdate {
				targetIdol.Images = append(targetIdol.Images, idol.Images...)

				_, err = updateBiasgame(idol.ID, targetIdol.ID)
				helpers.Relax(err)

				// delete current idol record
				err = helpers.MDbDelete(models.IdolTable, idol.ID)
				helpers.Relax(err)
			}

			// save target idol with new images
			err := helpers.MDbUpsertID(models.IdolTable, targetIdol.ID, targetIdol)
			helpers.Relax(err)
		}
	}

	// if a matching idol was NOT found and a target idol in mongo wasn't found, simply rename
	if matchingIdol == nil && !targetIdolFound {

		for _, idol := range idolsToUpdate {
			idol.Name = newName
			idol.GroupName = newGroup
			idol.Gender = newGender

			err := helpers.MDbUpsertID(models.IdolTable, idol.ID, idol)
			helpers.Relax(err)
		}
	}

	// update cache
	if len(GetAllIdols()) > 0 {
		setModuleCache(ALL_IDOLS_CACHE_KEY, GetAllIdols(), time.Hour*24*7)
	}

	return recordsFound
}

// updateImageInfo updates a specific image and its related idol info
func updateImageInfo(msg *discordgo.Message, content string) {
	cache.GetSession().ChannelTyping(msg.ChannelID)

	contentArgs, err := helpers.ToArgv(content)
	if err != nil {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.invalid"))
		return
	}
	contentArgs = contentArgs[1:]

	// confirm amount of args
	if len(contentArgs) < 4 {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.too-few"))
		return
	}

	targetObjectName := contentArgs[0]
	newGroup := contentArgs[1]
	newName := contentArgs[2]
	newGender := strings.ToLower(contentArgs[3])

	// if a gender was passed, make sure its valid
	if newGender != "boy" && newGender != "girl" {
		helpers.SendMessage(msg.ChannelID, "Invalid gender. Gender must be exactly 'girl' or 'boy'. No information was updated.")
		return
	}

	allIdols := GetAllIdols()
	allIdolsMutex.Lock()
	imageFound := false

	// find and delete target image by object name
IdolsLoop:
	for idolIndex, idol := range allIdols {

		// check if image has not been found and deleted, no need to loop through images if it has
		for i, img := range idol.Images {
			if img.ObjectName == targetObjectName {

				// IMPORTANT: it is important that we do not delete the last image from the idol AND the idol from the all idols array. it MUST be one OR the other.

				// if that was the last image for the idol, delete idol from all idols
				if len(idol.Images) == 1 {

					// remove pointer from array. struct will be garbage collected when not used by a game
					allIdols = append(allIdols[:idolIndex], allIdols[idolIndex+1:]...)
				} else {
					// delete image
					idol.Images = append(idol.Images[:i], idol.Images[i+1:]...)
				}
				imageFound = true
				break IdolsLoop
			}
		}
	}
	allIdolsMutex.Unlock()
	// update idols
	setAllIdols(allIdols)

	// confirm an image was found and deleted
	if !imageFound {
		helpers.SendMessage(msg.ChannelID, "No image with that object name was found. No information was updated.")
		return
	}

	// attempt to get matching idol
	groupCheck, nameCheck, idolToUpdate := GetMatchingIdolAndGroup(newGroup, newName, false)

	// get mdb record by object name
	var mongoRecordToUpdate models.IdolEntry
	err = helpers.MdbOne(helpers.MdbCollection(models.IdolTable).Find(bson.M{"images.objectname": targetObjectName}), &mongoRecordToUpdate)
	helpers.Relax(err)

	// if a database entry was found, update it
	if mongoRecordToUpdate.Name != "" {

		// get the image being updated, and remove the image from the mdb idol record
		var mdbImageRecord models.IdolImageEntry
		for imageIndex, mdbIdolImages := range mongoRecordToUpdate.Images {
			if mdbIdolImages.ObjectName == targetObjectName {
				mdbImageRecord = mdbIdolImages
				mongoRecordToUpdate.Images = append(mongoRecordToUpdate.Images[:imageIndex], mongoRecordToUpdate.Images[imageIndex+1:]...)
			}
		}

		// if the idol has no images left, delete it. else update it
		var deleteIdolId bson.ObjectId
		var err error
		if len(mongoRecordToUpdate.Images) == 0 {
			deleteIdolId = mongoRecordToUpdate.ID
			err = helpers.MDbDelete(models.IdolTable, mongoRecordToUpdate.ID)
		} else {
			err = helpers.MDbUpsertID(models.IdolTable, mongoRecordToUpdate.ID, mongoRecordToUpdate)
		}
		helpers.Relax(err)

		// if the new group/name already exists, add image to that idol. otherwise create new idol
		if groupCheck && nameCheck && idolToUpdate != nil {

			var targetIdol models.IdolEntry
			err = helpers.MdbOne(helpers.MdbCollection(models.IdolTable).Find(bson.M{"name": idolToUpdate.Name, "groupname": idolToUpdate.GroupName}), &targetIdol)
			helpers.Relax(err)

			targetIdol.Images = append(targetIdol.Images, mdbImageRecord)
			err := helpers.MDbUpsertID(models.IdolTable, targetIdol.ID, targetIdol)
			helpers.Relax(err)

			// if an idol was deleted update the biasgame stats to be for the new idol
			if deleteIdolId != "" {
				_, err = updateBiasgame(deleteIdolId, targetIdol.ID)
				helpers.Relax(err)
			}

			allIdolsMutex.Lock()
			idolToUpdate.Images = append(idolToUpdate.Images, IdolImage{
				ObjectName: mdbImageRecord.ObjectName,
				HashString: mdbImageRecord.HashString,
			})
			allIdolsMutex.Unlock()
		} else {

			newIdolEntry := models.IdolEntry{
				ID:        "",
				Name:      newName,
				GroupName: newGroup,
				Gender:    newGender,
				Images:    []models.IdolImageEntry{mdbImageRecord},
			}

			newIdolID, err := helpers.MDbInsert(models.IdolTable, newIdolEntry)
			helpers.Relax(err)
			newIdolEntry.ID = newIdolID

			newIdol := makeIdolFromIdolEntry(newIdolEntry)
			setAllIdols(append(GetAllIdols(), &newIdol))

			if deleteIdolId != "" {
				_, err = updateBiasgame(deleteIdolId, newIdol.ID)
				helpers.Relax(err)
			}
		}

	} else {
		// oh boy... these should not happen
		if mongoRecordToUpdate.Name != "" {
			helpers.SendMessage(msg.ChannelID, "No image with that object name was found IN MONGO, but the image was found memory. Data is out of sync, please refresh-images.")
		} else {
			helpers.SendMessage(msg.ChannelID, "To many images with that object name were found IN MONGO. This should never occur, please clean up the extra records manually and refresh-images")
		}
		return
	}

	// update cache
	if len(GetAllIdols()) > 0 {
		setModuleCache(ALL_IDOLS_CACHE_KEY, GetAllIdols(), time.Hour*24*7)
	}

	helpers.SendMessage(msg.ChannelID, fmt.Sprintf("Image Update. Object Name: %s | Idol: %s %s", targetObjectName, newGroup, newName))

}

// deleteImage deletes a image from an idol record. will delete idol as well if its the last image
func deleteImage(msg *discordgo.Message, content string) {
	contentArgs, err := helpers.ToArgv(content)
	if err != nil {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.invalid"))
		return
	}
	contentArgs = contentArgs[1:]

	// confirm amount of args
	if len(contentArgs) != 1 {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.invalid"))
		return
	}

	targetObjectName := contentArgs[0]

	allIdols := GetAllIdols()
	allIdolsMutex.Lock()
	imageFound := false

	// find and delete target image by object name
IdolLoop:
	for _, idol := range allIdols {

		// check if image has not been found and deleted, no need to loop through images if it has
		for i, bImg := range idol.Images {
			if bImg.ObjectName == targetObjectName {

				// IMPORTANT: it is important that we do not delete the last image from the idol AND the idol from the all idols array. it MUST be one OR the other.

				// if that was the last image for the idol, delete idol from all idols
				if len(idol.Images) == 1 {

					// if the whole idol is getting deleted, we need to load image
					//   bytes incase the image is being used by a game currently
					idol.Images[i].ImageBytes = idol.Images[i].GetImgBytes()
					idol.Deleted = true

				} else {
					// delete image
					idol.Images = append(idol.Images[:i], idol.Images[i+1:]...)
				}
				imageFound = true
				break IdolLoop
			}
		}
	}
	allIdolsMutex.Unlock()
	// update idols
	setAllIdols(allIdols)

	// confirm an image was found and deleted
	if !imageFound {
		helpers.SendMessage(msg.ChannelID, "No image with that object name was found.")
		return
	}

	// get mdb record by object name
	var mongoRecordToUpdate models.IdolEntry
	err = helpers.MdbOne(helpers.MdbCollection(models.IdolTable).Find(bson.M{"images.objectname": targetObjectName}), &mongoRecordToUpdate)
	helpers.Relax(err)

	// if a database entry were found, update it
	if mongoRecordToUpdate.Name != "" {

		// get the image being updated, and remove the image from the mdb idol record
		var mdbImageRecord models.IdolImageEntry
		for imageIndex, mdbIdolImages := range mongoRecordToUpdate.Images {
			if mdbIdolImages.ObjectName == targetObjectName {
				mdbImageRecord = mdbIdolImages
				mongoRecordToUpdate.Images = append(mongoRecordToUpdate.Images[:imageIndex], mongoRecordToUpdate.Images[imageIndex+1:]...)
			}
		}

		// if the idol has no images left, delete it. else update it
		var err error
		if len(mongoRecordToUpdate.Images) == 0 {

			// dont fully delete it because the idol is still referenced in biasgame and possibly other modules
			mongoRecordToUpdate.Deleted = true
			err = helpers.MDbUpsertID(models.IdolTable, mongoRecordToUpdate.ID, mongoRecordToUpdate)
		} else {
			err = helpers.MDbUpsertID(models.IdolTable, mongoRecordToUpdate.ID, mongoRecordToUpdate)
		}
		helpers.Relax(err)

		// delete object
		helpers.DeleteFile(mdbImageRecord.ObjectName)
	}

	// update cache
	if len(GetAllIdols()) > 0 {
		setModuleCache(ALL_IDOLS_CACHE_KEY, GetAllIdols(), time.Hour*24*7)
	}

	helpers.SendMessage(msg.ChannelID, fmt.Sprintf("Deleted image with object name: %s", targetObjectName))

}

// setAllIdols setter for all idols
func setAllIdols(idols []*Idol) {
	allIdolsMutex.Lock()
	allIdolsMutex.Unlock()

	allIdols = idols

	// set active idols
	activeIdols = nil
	for _, idol := range idols {
		if idol.Deleted == false && len(idol.Images) != 0 {
			activeIdols = append(activeIdols, idol)
		}
	}
}

// showImagesForIdol will show a embed message with all the available images for an idol
func showImagesForIdol(msg *discordgo.Message, msgContent string, showObjectNames bool) {
	defer helpers.Recover()
	cache.GetSession().ChannelTyping(msg.ChannelID)

	commandArgs, err := helpers.ToArgv(msgContent)
	if err != nil {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.invalid"))
		return
	}
	commandArgs = commandArgs[1:]

	if len(commandArgs) < 2 {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.too-few"))
		return
	}

	// get matching idol to the group and name entered
	//  if we can't get one display an error
	groupMatch, nameMatch, matchIdol := GetMatchingIdolAndGroup(commandArgs[0], commandArgs[1], true)
	if matchIdol == nil || groupMatch == false || nameMatch == false {
		helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.biasgame.stats.no-matching-idol"))
		return
	}

	// get bytes of all the images
	var idolImages []IdolImage
	for _, bImag := range matchIdol.Images {
		idolImages = append(idolImages, bImag)
	}

	sendPagedEmbedOfImages(msg, idolImages, showObjectNames,
		fmt.Sprintf("Images for %s %s", matchIdol.GroupName, matchIdol.Name),
		fmt.Sprintf("Total Images: %s", humanize.Comma(int64(len(matchIdol.Images)))))
}

// listIdols will list all idols
func listIdols(msg *discordgo.Message) {
	cache.GetSession().ChannelTyping(msg.ChannelID)

	genderCountMap := make(map[string]int)
	genderGroupCountMap := make(map[string]int)

	// create map of idols and there group
	groupIdolMap := make(map[string][]string)
	for _, idol := range GetActiveIdols() {

		// count idols and groups
		genderCountMap[idol.Gender]++
		if _, ok := groupIdolMap[idol.GroupName]; !ok {
			genderGroupCountMap[idol.Gender]++
		}

		if len(idol.Images) > 1 {
			groupIdolMap[idol.GroupName] = append(groupIdolMap[idol.GroupName], fmt.Sprintf("%s (%s)",
				idol.Name, humanize.Comma(int64(len(idol.Images)))))
		} else {

			groupIdolMap[idol.GroupName] = append(groupIdolMap[idol.GroupName], fmt.Sprintf("%s", idol.Name))
		}
	}

	embed := &discordgo.MessageEmbed{
		Color: 0x0FADED, // blueish
		Author: &discordgo.MessageEmbedAuthor{
			Name: "All Idols Available",
		},
		Title: fmt.Sprintf("%s Total | %s Girls, %s Boys | %s Girl Groups, %s Boy Groups",
			humanize.Comma(int64(len(GetActiveIdols()))),
			humanize.Comma(int64(genderCountMap["girl"])),
			humanize.Comma(int64(genderCountMap["boy"])),
			humanize.Comma(int64(genderGroupCountMap["girl"])),
			humanize.Comma(int64(genderGroupCountMap["boy"])),
		),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Numbers show idols picture count",
		},
	}

	// make fields for each group and the idols in the group.
	for group, idols := range groupIdolMap {

		// sort idols by name
		sort.Slice(idols, func(i, j int) bool {
			return idols[i] < idols[j]
		})

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   group,
			Value:  strings.Join(idols, ", "),
			Inline: false,
		})
	}

	// sort fields by group name
	sort.Slice(embed.Fields, func(i, j int) bool {
		return strings.ToLower(embed.Fields[i].Name) < strings.ToLower(embed.Fields[j].Name)
	})

	helpers.SendPagedMessage(msg, embed, 10)
}

// updateBiasgame update the idol id for biasgame records
//  this can't be included in the biasgame package because of circular imports
func updateBiasgame(targetId, newId bson.ObjectId) (int, error) {

	// update biasgame stats
	// update is done in pairs, first the select query, and then the update.
	updateArray := []interface{}{
		bson.M{"gamewinner": targetId},
		bson.M{"$set": bson.M{"gamewinner": newId}},

		bson.M{"roundwinners": targetId},
		bson.M{"$set": bson.M{"roundwinners.$": newId}},

		bson.M{"roundlosers": targetId},
		bson.M{"$set": bson.M{"roundlosers.$": newId}},
	}

	modified := 0

	// update in a loop as $ is a positional operator and therefore not all array elements for the round will be updated immediatly. loop through and update them until completed
	//   wish this wasn't needed but mgo doesn't have a proper way to do arrayfilter with update multi mongo operation
	for true {

		// run bulk operation to update records
		bulkOperation := helpers.MdbCollection(models.BiasGameTable).Bulk()
		bulkOperation.UpdateAll(updateArray...)
		bulkResults, err := bulkOperation.Run()
		if err != nil {
			log().Errorln("Bulk update error: ", err.Error())
			return 0, err
		}

		modified += bulkResults.Modified

		// break when no more records are being modified
		if bulkResults.Modified == 0 {
			break
		}
	}

	return modified, nil
}

// RefreshIdolBiasgameStats is a slow operation that will check every game the
// idol has ever been in and update its game stats
func RefreshIdolBiasgameStats(targetIdol *Idol) {

	// get all the games that the target idol has been in
	queryParams := bson.M{"$or": []bson.M{
		// check if idol is in round winner or losers array
		{"roundwinners": targetIdol.ID},
		{"roundlosers": targetIdol.ID},
	}}

	// query db for information on this
	var targetIdolGames []models.BiasGameEntry
	helpers.MDbIter(helpers.MdbCollection(models.BiasGameTable).Find(queryParams)).All(&targetIdolGames)

	totalGames := len(targetIdolGames)
	totalGameWins := 0
	totalRounds := 0
	totalRoundWins := 0

	for _, game := range targetIdolGames {

		// win game
		if game.GameWinner == targetIdol.ID {
			totalGameWins++
		}

		// round win
		for _, roundWinnerId := range game.RoundWinners {
			if roundWinnerId == targetIdol.ID {
				totalRounds++
				totalRoundWins++
			}
		}
		// round lose
		for _, roundLoserId := range game.RoundLosers {
			if roundLoserId == targetIdol.ID {
				totalRounds++
			}
		}
	}

	targetIdol.BGGames = totalGames
	targetIdol.BGGameWins = totalGameWins
	targetIdol.BGRounds = totalRounds
	targetIdol.BGRoundWins = totalRoundWins

	// update cache
	if len(GetAllIdols()) > 0 {
		setModuleCache(ALL_IDOLS_CACHE_KEY, GetAllIdols(), time.Hour*24*7)
	}
}

// RefreshIdolBiasgameStats is a slow operation that will check every game the
// idol has ever been in and update its game stats
func RefreshAllIdolBiasgameStats(msg *discordgo.Message) {
	cache.GetSession().ChannelTyping(msg.ChannelID)

	helpers.SendMessage(msg.ChannelID, "Refreshing biasgame info on idol records...")

	var allGames []models.BiasGameEntry
	helpers.MDbIter(helpers.MdbCollection(models.BiasGameTable).Find(bson.M{})).All(&allGames)

	totalIdols := len(GetAllIdols())
	helpers.SendMessage(msg.ChannelID, fmt.Sprintln("Total games found: ", len(allGames)))
	helpers.SendMessage(msg.ChannelID, fmt.Sprintln("Idols found: ", totalIdols))
	helpers.SendMessage(msg.ChannelID, "Processing...")

	idolsProcessed := 0
	log().Infof("Idols processed: %d / %d", idolsProcessed, totalIdols)

	idolGameTotalMap := make(map[bson.ObjectId]int)
	idolGameWinMap := make(map[bson.ObjectId]int)
	idolRoundTotalMap := make(map[bson.ObjectId]int)
	idolRoundWinMap := make(map[bson.ObjectId]int)

	for _, game := range allGames {

		idolsFound := make(map[bson.ObjectId]bool)
		idolsFound[game.GameWinner] = true

		idolGameWinMap[game.GameWinner]++

		// round win
		for _, roundWinnerId := range game.RoundWinners {
			idolRoundWinMap[roundWinnerId]++
			idolRoundTotalMap[roundWinnerId]++
			idolsFound[roundWinnerId] = true

		}
		// round lose
		for _, roundLoserId := range game.RoundLosers {
			idolRoundTotalMap[roundLoserId]++
			idolsFound[roundLoserId] = true
		}

		for idolId, _ := range idolsFound {
			idolGameTotalMap[idolId]++
		}
	}

	for _, targetIdol := range GetAllIdols() {

		log().Infoln("Running for: ", targetIdol.GroupName, targetIdol.Name, idolGameTotalMap[targetIdol.ID])

		targetIdol.BGGames = idolGameTotalMap[targetIdol.ID]
		targetIdol.BGGameWins = idolGameWinMap[targetIdol.ID]
		targetIdol.BGRounds = idolRoundTotalMap[targetIdol.ID]
		targetIdol.BGRoundWins = idolRoundWinMap[targetIdol.ID]

		idolsProcessed++
		log().Infof("Idols processed: %d / %d", idolsProcessed, totalIdols)

		var targetIdolEntry models.IdolEntry
		err := helpers.MdbOne(helpers.MdbCollection(models.IdolTable).Find(bson.M{"groupname": targetIdol.GroupName, "name": targetIdol.Name}), &targetIdolEntry)
		helpers.Relax(err)

		targetIdolEntry.BGGames = targetIdol.BGGames
		targetIdolEntry.BGGameWins = targetIdol.BGGameWins
		targetIdolEntry.BGRounds = targetIdol.BGRounds
		targetIdolEntry.BGRoundWins = targetIdol.BGRoundWins
		err = helpers.MDbUpsertID(models.IdolTable, targetIdolEntry.ID, targetIdolEntry)
		helpers.Relax(err)
	}

	// update cache
	if len(GetAllIdols()) > 0 {
		setModuleCache(ALL_IDOLS_CACHE_KEY, GetAllIdols(), time.Hour*24*7)
	}
	helpers.SendMessage(msg.ChannelID, "Done.")
}

// UpdateIdolGameStats is called every time a biasgame finished
func UpdateIdolGameStats(game models.BiasGameEntry) {
	helpers.Recover()

	roundWinIdols := make(map[bson.ObjectId]int)
	roundTotalIdols := make(map[bson.ObjectId]int)

	for _, roundWinner := range game.RoundWinners {
		roundWinIdols[roundWinner]++
		roundTotalIdols[roundWinner]++
	}

	for _, roundWinner := range game.RoundLosers {
		roundTotalIdols[roundWinner]++
	}

	for _, idol := range GetActiveIdols() {
		inGame := false
		if idol.ID == game.GameWinner {
			idol.BGGameWins++
			inGame = true
		}

		if roundWins, ok := roundWinIdols[idol.ID]; ok {
			idol.BGRoundWins = idol.BGRoundWins + roundWins
			inGame = true
		}

		if roundTotal, ok := roundTotalIdols[idol.ID]; ok {
			idol.BGRounds = idol.BGRounds + roundTotal
			inGame = true
		}

		if inGame {
			idol.BGGames++

			var targetIdolEntry models.IdolEntry
			err := helpers.MdbOne(helpers.MdbCollection(models.IdolTable).Find(bson.M{"groupname": idol.GroupName, "name": idol.Name}), &targetIdolEntry)
			helpers.Relax(err)

			targetIdolEntry.BGGames = idol.BGGames
			targetIdolEntry.BGGameWins = idol.BGGameWins
			targetIdolEntry.BGRounds = idol.BGRounds
			targetIdolEntry.BGRoundWins = idol.BGRoundWins
			err = helpers.MDbUpsertID(models.IdolTable, targetIdolEntry.ID, targetIdolEntry)
			helpers.Relax(err)
		}
	}

	// update cache
	if len(GetAllIdols()) > 0 {
		setModuleCache(ALL_IDOLS_CACHE_KEY, GetAllIdols(), time.Hour*24*7)
	}
}
